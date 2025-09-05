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

// MARK: NewUpdateManager
func NewUpdateManager(cfg *config.Config, logger *internal.Logger, currentVersion string) *UpdateManager {
	ctx, cancel := context.WithCancel(context.Background())
	backupDir := cfg.Update.BackupDir
	if backupDir == "" {
		backupDir = DefaultBackupDir
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
		u.logger.Info("Auto-update disabled")
		return nil
	}

	schedule := u.cfg.Update.Schedule
	if schedule == "" {
		schedule = DefaultUpdateCheckInterval
	}
	if err := os.MkdirAll(u.backupDir, 0755); err != nil {
		return fmt.Errorf("creating backup dir: %w", err)
	}
	if err := u.scheduler.Start(schedule, u.performScheduledUpdate); err != nil {
		return fmt.Errorf("starting scheduler: %w", err)
	}

	u.logger.Info("Auto-update started",
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
	u.logger.Info("Auto-update stopped")
	return nil
}

// MARK: CheckForUpdates
func (u *UpdateManager) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	u.lastCheckTime = time.Now()
	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	if release.Draft || release.Prerelease {
		u.logger.Debug("Skipping draft/prerelease", "version", release.TagName)
		return u.buildUpdateInfo(false, release.TagName, ""), nil
	}

	available := u.isNewerVersion(release.TagName, u.currentVersion)
	info := u.buildUpdateInfo(available, release.TagName, release.Body)

	if available {
		u.logger.Info("Update available", "current", u.currentVersion, "latest", release.TagName)
	} else {
		u.logger.Debug("No updates", "current", u.currentVersion)
	}
	return info, nil
}

// MARK: PerformUpdate
func (u *UpdateManager) PerformUpdate(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&u.updateInProgress, 0, 1) {
		return fmt.Errorf("update in progress")
	}
	defer atomic.StoreInt64(&u.updateInProgress, 0)

	u.logger.Info("Starting update")
	ctx, cancel := context.WithTimeout(ctx, UpdateTimeout)
	defer cancel()

	sm := NewServiceManager("finguard", u.logger)
	if err := sm.ValidatePermissions(); err != nil {
		return fmt.Errorf("permission validation failed: %w", err)
	}

	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("fetching release: %w", err)
	}
	if !u.isNewerVersion(release.TagName, u.currentVersion) {
		return fmt.Errorf("no newer version")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	if err := u.createBackup(); err != nil {
		u.logger.Error("Backup failed", "error", err)
	}

	u.logger.Info("Downloading binary")
	tmpPath, err := u.downloadAndExtractBinary(ctx, release)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpPath)

	oldPath := execPath + ".old"
	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("moving old binary: %w", err)
	}
	defer os.Remove(oldPath)

	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Rename(oldPath, execPath)
		return fmt.Errorf("placing new binary: %w", err)
	}
	_ = os.Chmod(execPath, 0755)
	_ = os.Chown(execPath, os.Getuid(), os.Getgid())
	sm.SetCapabilities(execPath)

	u.logger.Info("Update completed",
		"old_version", u.currentVersion,
		"new_version", release.TagName)

	go func() {
		time.Sleep(2 * time.Second)
		u.logger.Info("Exiting for systemd restart")
		os.Exit(0)
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
		u.logger.Warn("Saving schedule failed", "error", err)
	}
	u.logger.Info("Schedule updated", "new_schedule", schedule)
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
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
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
	v1Parts, v2Parts := strings.Split(v1, "."), strings.Split(v2, ".")
	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}
	for i := 0; i < maxLen; i++ {
		p1, p2 := "0", "0"
		if i < len(v1Parts) {
			p1 = strings.Split(v1Parts[i], "-")[0]
		}
		if i < len(v2Parts) {
			p2 = strings.Split(v2Parts[i], "-")[0]
		}
		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}
	return 0
}

// MARK: createBackup
func (u *UpdateManager) createBackup() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	backupName := fmt.Sprintf("finguard_%s_%s.backup",
		u.currentVersion, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(u.backupDir, backupName)

	src, err := os.Open(execPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()
	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying binary: %w", err)
	}
	if err := dst.Chmod(0755); err != nil {
		return fmt.Errorf("chmod backup: %w", err)
	}
	u.logger.Info("Backup created", "path", backupPath)
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
		return "", fmt.Errorf("no binary for %s", assetName)
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
		return "", fmt.Errorf("creating temp dir: %w", err)
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
		return "", fmt.Errorf("chmod binary: %w", err)
	}
	return binaryPath, nil
}

// MARK: extractTarGz
func (u *UpdateManager) extractTarGz(reader io.Reader, destDir string) (string, error) {
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer gzReader.Close()
	tarReader := tar.NewReader(gzReader)

	for {
		h, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		fileName := filepath.Base(h.Name)
		if strings.HasPrefix(fileName, "finguard") &&
			(runtime.GOOS != "windows" || strings.HasSuffix(fileName, ".exe")) {
			binaryPath := filepath.Join(destDir, fileName)
			dst, err := os.Create(binaryPath)
			if err != nil {
				return "", fmt.Errorf("create extracted: %w", err)
			}
			if _, err := io.Copy(dst, tarReader); err != nil {
				dst.Close()
				return "", fmt.Errorf("extracting binary: %w", err)
			}
			if err := dst.Chmod(0755); err != nil {
				dst.Close()
				return "", fmt.Errorf("chmod extracted: %w", err)
			}
			dst.Close()
			return binaryPath, nil
		}
	}
	return "", fmt.Errorf("binary not found in archive")
}

// MARK: getBinaryAssetName
func (u *UpdateManager) getBinaryAssetName() string {
	arch := runtime.GOARCH
	os := runtime.GOOS
	return fmt.Sprintf("%s-%s", os, arch)
}

// MARK: performScheduledUpdate
func (u *UpdateManager) performScheduledUpdate() {
	if !u.cfg.Update.AutoApply {
		u.logger.Info("Scheduled check (auto-apply disabled)")
		if _, err := u.CheckForUpdates(u.ctx); err != nil {
			u.logger.Error("Scheduled check failed", "error", err)
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
