package updater

import (
	"context"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

// MARK: ServiceManager
type ServiceManager struct {
	serviceName string
	isSystemd   bool
	logger      *internal.Logger
}

// MARK: CronScheduler
type CronScheduler struct {
	logger   *internal.Logger
	schedule string
	nextRun  time.Time
	running  int64
	ctx      context.Context
	cancel   context.CancelFunc
	taskFunc func()
}

// MARK: CronEntry
type CronEntry struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int
}

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
