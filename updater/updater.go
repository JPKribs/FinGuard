package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

const (
	DefaultUpdateCheckInterval = "0 3 * * *"
	GitHubReleasesAPI          = "https://api.github.com/repos/JPKribs/FinGuard/releases/latest"
	DefaultBackupDir           = "./backups"
	UpdateTimeout              = 10 * time.Minute
)

// MARK: UpdateManager
type UpdateManager struct {
	cfg              *config.Config
	logger           *internal.Logger
	currentVersion   string
	lastCheckTime    time.Time
	updateInProgress int64
	ctx              context.Context
	cancel           context.CancelFunc
	scheduler        *CronScheduler
	backupDir        string
	repoOwner        string
	repoName         string
}

// MARK: UpdateInfo
type UpdateInfo struct {
	Available         bool      `json:"available"`
	CurrentVersion    string    `json:"current_version"`
	LatestVersion     string    `json:"latest_version"`
	ReleaseNotes      string    `json:"release_notes"`
	LastCheckTime     time.Time `json:"last_check_time"`
	NextCheckTime     time.Time `json:"next_check_time"`
	UpdateSchedule    string    `json:"update_schedule"`
	AutoUpdateEnabled bool      `json:"auto_update_enabled"`
}

// MARK: GitHubRelease
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// MARK: NewUpdateManager
func NewUpdateManager(cfg *config.Config, logger *internal.Logger, currentVersion string) *UpdateManager {
	ctx, cancel := context.WithCancel(context.Background())

	backupDir := DefaultBackupDir
	if cfg.Update.BackupDir != "" {
		backupDir = cfg.Update.BackupDir
	}

	return &UpdateManager{
		cfg:            cfg,
		logger:         logger,
		currentVersion: currentVersion,
		ctx:            ctx,
		cancel:         cancel,
		backupDir:      backupDir,
		repoOwner:      "JPKribs",
		repoName:       "FinGuard",
		scheduler:      NewCronScheduler(logger),
	}
}

// MARK: Start
func (u *UpdateManager) Start() error {
	if !u.cfg.Update.Enabled {
		u.logger.Info("Auto-update is disabled")
		return nil
	}

	schedule := u.cfg.Update.Schedule
	if schedule == "" {
		schedule = DefaultUpdateCheckInterval
	}

	if err := os.MkdirAll(u.backupDir, 0755); err != nil {
		return fmt.Errorf("creating backup directory: %w", err)
	}

	if err := u.scheduler.Start(schedule, u.performScheduledUpdate); err != nil {
		return fmt.Errorf("starting update scheduler: %w", err)
	}

	u.logger.Info("Auto-update system started",
		"schedule", schedule,
		"current_version", u.currentVersion,
		"backup_dir", u.backupDir)

	return nil
}

// MARK: Stop
func (u *UpdateManager) Stop() error {
	u.cancel()
	if u.scheduler != nil {
		u.scheduler.Stop()
	}
	u.logger.Info("Auto-update system stopped")
	return nil
}

// MARK: CheckForUpdates
func (u *UpdateManager) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	u.lastCheckTime = time.Now()

	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}

	if release.Draft || release.Prerelease {
		u.logger.Debug("Skipping draft/prerelease version", "version", release.TagName)
		return u.buildUpdateInfo(false, release.TagName, ""), nil
	}

	available := u.isNewerVersion(release.TagName, u.currentVersion)
	info := u.buildUpdateInfo(available, release.TagName, release.Body)

	if available {
		u.logger.Info("Update available",
			"current", u.currentVersion,
			"latest", release.TagName)
	} else {
		u.logger.Debug("No updates available", "current", u.currentVersion)
	}

	return info, nil
}

// MARK: PerformUpdate
func (u *UpdateManager) PerformUpdate(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&u.updateInProgress, 0, 1) {
		return fmt.Errorf("update already in progress")
	}
	defer atomic.StoreInt64(&u.updateInProgress, 0)

	u.logger.Info("Starting application update")

	ctx, cancel := context.WithTimeout(ctx, UpdateTimeout)
	defer cancel()

	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("fetching release info: %w", err)
	}

	if !u.isNewerVersion(release.TagName, u.currentVersion) {
		return fmt.Errorf("no newer version available")
	}

	if err := u.createBackup(); err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}

	binaryPath, err := u.downloadAndExtractBinary(ctx, release)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}

	if err := u.replaceBinary(binaryPath); err != nil {
		if restoreErr := u.restoreFromBackup(); restoreErr != nil {
			u.logger.Error("Failed to restore from backup", "error", restoreErr)
		}
		return fmt.Errorf("replacing binary: %w", err)
	}

	u.logger.Info("Update completed successfully",
		"old_version", u.currentVersion,
		"new_version", release.TagName)

	go func() {
		time.Sleep(2 * time.Second)
		u.restartApplication()
	}()

	return nil
}

// MARK: GetUpdateStatus
func (u *UpdateManager) GetUpdateStatus() UpdateInfo {
	nextCheck := time.Time{}
	if u.scheduler != nil {
		nextCheck = u.scheduler.NextRun()
	}

	return UpdateInfo{
		Available:         false,
		CurrentVersion:    u.currentVersion,
		LatestVersion:     u.currentVersion,
		LastCheckTime:     u.lastCheckTime,
		NextCheckTime:     nextCheck,
		UpdateSchedule:    u.cfg.Update.Schedule,
		AutoUpdateEnabled: u.cfg.Update.Enabled,
	}
}

// MARK: UpdateSchedule
func (u *UpdateManager) UpdateSchedule(schedule string) error {
	if u.scheduler == nil {
		return fmt.Errorf("scheduler not initialized")
	}

	if err := u.scheduler.UpdateSchedule(schedule); err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}

	u.cfg.Update.Schedule = schedule
	if err := u.cfg.SaveUpdate(); err != nil {
		u.logger.Warn("Failed to save updated schedule", "error", err)
	}

	u.logger.Info("Update schedule changed", "new_schedule", schedule)
	return nil
}

// MARK: fetchLatestRelease
func (u *UpdateManager) fetchLatestRelease(ctx context.Context) (*GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", GitHubReleasesAPI, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", fmt.Sprintf("FinGuard/%s", u.currentVersion))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &release, nil
}

// MARK: isNewerVersion
func (u *UpdateManager) isNewerVersion(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	return u.compareVersions(latest, current) > 0
}

// MARK: compareVersions
func (u *UpdateManager) compareVersions(v1, v2 string) int {
	v1Parts := strings.Split(v1, ".")
	v2Parts := strings.Split(v2, ".")

	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 string
		if i < len(v1Parts) {
			p1 = v1Parts[i]
		} else {
			p1 = "0"
		}
		if i < len(v2Parts) {
			p2 = v2Parts[i]
		} else {
			p2 = "0"
		}

		p1 = strings.Split(p1, "-")[0]
		p2 = strings.Split(p2, "-")[0]

		if p1 > p2 {
			return 1
		} else if p1 < p2 {
			return -1
		}
	}

	return 0
}

// MARK: createBackup
func (u *UpdateManager) createBackup() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	backupName := fmt.Sprintf("finguard_%s_%s.backup",
		u.currentVersion,
		time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(u.backupDir, backupName)

	src, err := os.Open(execPath)
	if err != nil {
		return fmt.Errorf("opening source binary: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying binary: %w", err)
	}

	if err := dst.Chmod(0755); err != nil {
		return fmt.Errorf("setting backup permissions: %w", err)
	}

	u.logger.Info("Created backup", "path", backupPath)
	return nil
}

// MARK: downloadAndExtractBinary
func (u *UpdateManager) downloadAndExtractBinary(ctx context.Context, release *GitHubRelease) (string, error) {
	assetName := u.getBinaryAssetName()
	var downloadURL string

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, assetName) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no suitable binary found for %s", assetName)
	}

	u.logger.Info("Downloading update", "url", downloadURL)

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempDir := filepath.Join(u.backupDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}

	if strings.HasSuffix(downloadURL, ".tar.gz") {
		return u.extractTarGz(resp.Body, tempDir)
	}

	binaryPath := filepath.Join(tempDir, "finguard")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	dst, err := os.Create(binaryPath)
	if err != nil {
		return "", fmt.Errorf("creating binary file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, resp.Body); err != nil {
		return "", fmt.Errorf("downloading binary: %w", err)
	}

	if err := dst.Chmod(0755); err != nil {
		return "", fmt.Errorf("setting binary permissions: %w", err)
	}

	return binaryPath, nil
}

// MARK: extractTarGz
func (u *UpdateManager) extractTarGz(reader io.Reader, destDir string) (string, error) {
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	var binaryPath string

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		fileName := filepath.Base(header.Name)
		if strings.HasPrefix(fileName, "finguard") && (runtime.GOOS != "windows" || strings.HasSuffix(fileName, ".exe")) {
			binaryPath = filepath.Join(destDir, fileName)

			dst, err := os.Create(binaryPath)
			if err != nil {
				return "", fmt.Errorf("creating extracted file: %w", err)
			}

			if _, err := io.Copy(dst, tarReader); err != nil {
				dst.Close()
				return "", fmt.Errorf("extracting binary: %w", err)
			}

			if err := dst.Chmod(0755); err != nil {
				dst.Close()
				return "", fmt.Errorf("setting permissions: %w", err)
			}

			dst.Close()
			break
		}
	}

	if binaryPath == "" {
		return "", fmt.Errorf("finguard binary not found in archive")
	}

	return binaryPath, nil
}

// MARK: getBinaryAssetName
func (u *UpdateManager) getBinaryAssetName() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	}

	os := runtime.GOOS
	if os == "darwin" {
		os = "darwin"
	}

	return fmt.Sprintf("%s-%s", os, arch)
}

// MARK: replaceBinary
func (u *UpdateManager) replaceBinary(newBinaryPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	tempPath := execPath + ".new"

	if err := u.copyFile(newBinaryPath, tempPath); err != nil {
		return fmt.Errorf("copying new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("backing up current binary: %w", err)
		}

		if err := os.Rename(tempPath, execPath); err != nil {
			os.Rename(oldPath, execPath)
			return fmt.Errorf("installing new binary: %w", err)
		}

		os.Remove(oldPath)
	} else {
		if err := os.Rename(tempPath, execPath); err != nil {
			return fmt.Errorf("installing new binary: %w", err)
		}
	}

	u.logger.Info("Binary updated successfully", "path", execPath)
	return nil
}

// MARK: copyFile
func (u *UpdateManager) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	return dstFile.Chmod(srcInfo.Mode())
}

// MARK: restoreFromBackup
func (u *UpdateManager) restoreFromBackup() error {
	backupFiles, err := filepath.Glob(filepath.Join(u.backupDir, "finguard_*.backup"))
	if err != nil || len(backupFiles) == 0 {
		return fmt.Errorf("no backup files found")
	}

	latestBackup := backupFiles[len(backupFiles)-1]

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	if err := u.copyFile(latestBackup, execPath); err != nil {
		return fmt.Errorf("restoring from backup: %w", err)
	}

	u.logger.Info("Restored from backup", "backup", latestBackup)
	return nil
}

// MARK: restartApplication
func (u *UpdateManager) restartApplication() {
	u.logger.Info("Restarting application after update")

	execPath, err := os.Executable()
	if err != nil {
		u.logger.Error("Failed to get executable path for restart", "error", err)
		return
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		u.logger.Error("Failed to start new process", "error", err)
		return
	}

	u.logger.Info("New process started, shutting down current instance")
	os.Exit(0)
}

// MARK: performScheduledUpdate
func (u *UpdateManager) performScheduledUpdate() {
	if !u.cfg.Update.AutoApply {
		u.logger.Info("Scheduled update check (auto-apply disabled)")
		if _, err := u.CheckForUpdates(u.ctx); err != nil {
			u.logger.Error("Scheduled update check failed", "error", err)
		}
		return
	}

	u.logger.Info("Performing scheduled update")
	if err := u.PerformUpdate(u.ctx); err != nil {
		u.logger.Error("Scheduled update failed", "error", err)
	}
}

// MARK: buildUpdateInfo
func (u *UpdateManager) buildUpdateInfo(available bool, latestVersion, releaseNotes string) *UpdateInfo {
	nextCheck := time.Time{}
	if u.scheduler != nil {
		nextCheck = u.scheduler.NextRun()
	}

	return &UpdateInfo{
		Available:         available,
		CurrentVersion:    u.currentVersion,
		LatestVersion:     latestVersion,
		ReleaseNotes:      releaseNotes,
		LastCheckTime:     u.lastCheckTime,
		NextCheckTime:     nextCheck,
		UpdateSchedule:    u.cfg.Update.Schedule,
		AutoUpdateEnabled: u.cfg.Update.Enabled,
	}
}
