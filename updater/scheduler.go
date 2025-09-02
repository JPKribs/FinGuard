package updater

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JPKribs/FinGuard/internal"
)

// MARK: NewCronScheduler
// Creates a new CronScheduler instance with a logger
func NewCronScheduler(logger *internal.Logger) *CronScheduler {
	return &CronScheduler{
		logger: logger,
	}
}

// MARK: Start
// Starts the cron scheduler with the given schedule and task function
func (c *CronScheduler) Start(schedule string, taskFunc func()) error {
	if atomic.LoadInt64(&c.running) == 1 {
		return fmt.Errorf("scheduler already running")
	}

	entry, err := c.parseSchedule(schedule)
	if err != nil {
		return fmt.Errorf("parsing schedule: %w", err)
	}

	c.schedule = schedule
	c.taskFunc = taskFunc
	c.nextRun = c.calculateNextRun(entry, time.Now())

	c.ctx, c.cancel = context.WithCancel(context.Background())
	atomic.StoreInt64(&c.running, 1)

	go c.run(entry)

	c.logger.Info("Cron scheduler started",
		"schedule", schedule,
		"next_run", c.nextRun.Format(time.RFC3339))

	return nil
}

// MARK: Stop
// Stops the currently running cron scheduler
func (c *CronScheduler) Stop() {
	if !atomic.CompareAndSwapInt64(&c.running, 1, 0) {
		return
	}

	if c.cancel != nil {
		c.cancel()
	}

	c.logger.Info("Cron scheduler stopped")
}

// MARK: NextRun
// Returns the next scheduled run time
func (c *CronScheduler) NextRun() time.Time {
	return c.nextRun
}

// MARK: UpdateSchedule
// Updates the cron schedule; restarts scheduler if it was running
func (c *CronScheduler) UpdateSchedule(schedule string) error {
	wasRunning := atomic.LoadInt64(&c.running) == 1

	if wasRunning {
		c.Stop()
	}

	if wasRunning {
		return c.Start(schedule, c.taskFunc)
	}

	c.schedule = schedule
	return nil
}

// MARK: run
// Internal loop to execute the scheduled task when the next run time is reached
func (c *CronScheduler) run(entry *CronEntry) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case now := <-ticker.C:
			now = now.Truncate(time.Minute)

			if now.Equal(c.nextRun) || now.After(c.nextRun) {
				c.logger.Info("Executing scheduled task", "scheduled_time", c.nextRun)

				go func() {
					defer func() {
						if r := recover(); r != nil {
							c.logger.Error("Scheduled task panicked", "error", r)
						}
					}()
					c.taskFunc()
				}()

				c.nextRun = c.calculateNextRun(entry, now)
				c.logger.Debug("Next scheduled run", "next_run", c.nextRun)
			}
		}
	}
}

// MARK: parseSchedule
// Parses a cron schedule string into a CronEntry structure
func (c *CronScheduler) parseSchedule(schedule string) (*CronEntry, error) {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron format, expected 5 fields (minute hour day month weekday), got %d", len(fields))
	}

	entry := &CronEntry{}
	var err error

	entry.Minute, err = c.parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("parsing minute: %w", err)
	}

	entry.Hour, err = c.parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("parsing hour: %w", err)
	}

	entry.DayOfMonth, err = c.parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("parsing day of month: %w", err)
	}

	entry.Month, err = c.parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("parsing month: %w", err)
	}

	entry.DayOfWeek, err = c.parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("parsing day of week: %w", err)
	}

	return entry, nil
}

// MARK: parseField
// Parses individual cron fields, supporting ranges and step values
func (c *CronScheduler) parseField(field string, min, max int) ([]int, error) {
	if field == "*" {
		result := make([]int, max-min+1)
		for i := range result {
			result[i] = min + i
		}
		return result, nil
	}

	var result []int
	parts := strings.Split(field, ",")

	for _, part := range parts {
		if strings.Contains(part, "/") {
			stepParts := strings.Split(part, "/")
			if len(stepParts) != 2 {
				return nil, fmt.Errorf("invalid step format: %s", part)
			}

			step, err := strconv.Atoi(stepParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid step value: %s", stepParts[1])
			}

			rangePart := stepParts[0]
			if rangePart == "*" {
				for i := min; i <= max; i += step {
					result = append(result, i)
				}
			} else if strings.Contains(rangePart, "-") {
				rangeValues, err := c.parseRange(rangePart, min, max)
				if err != nil {
					return nil, err
				}
				for i := rangeValues[0]; i <= rangeValues[len(rangeValues)-1]; i += step {
					result = append(result, i)
				}
			} else {
				start, err := strconv.Atoi(rangePart)
				if err != nil {
					return nil, fmt.Errorf("invalid start value: %s", rangePart)
				}
				if start < min || start > max {
					return nil, fmt.Errorf("value %d out of range [%d, %d]", start, min, max)
				}
				for i := start; i <= max; i += step {
					result = append(result, i)
				}
			}
		} else if strings.Contains(part, "-") {
			rangeValues, err := c.parseRange(part, min, max)
			if err != nil {
				return nil, err
			}
			result = append(result, rangeValues...)
		} else {
			value, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			if value < min || value > max {
				return nil, fmt.Errorf("value %d out of range [%d, %d]", value, min, max)
			}
			result = append(result, value)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid values parsed from: %s", field)
	}

	return result, nil
}

// MARK: parseRange
// Parses a range string (e.g. "1-5") into a slice of integers
func (c *CronScheduler) parseRange(rangeStr string, min, max int) ([]int, error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range format: %s", rangeStr)
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid range start: %s", parts[0])
	}

	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid range end: %s", parts[1])
	}

	if start < min || start > max || end < min || end > max {
		return nil, fmt.Errorf("range [%d, %d] out of bounds [%d, %d]", start, end, min, max)
	}

	if start > end {
		return nil, fmt.Errorf("invalid range: start %d > end %d", start, end)
	}

	result := make([]int, end-start+1)
	for i := range result {
		result[i] = start + i
	}

	return result, nil
}

// MARK: calculateNextRun
// Computes the next scheduled run time based on a CronEntry and reference time
func (c *CronScheduler) calculateNextRun(entry *CronEntry, from time.Time) time.Time {
	next := from.Add(1 * time.Minute).Truncate(time.Minute)

	for i := 0; i < 366*24*60; i++ {
		if c.matches(entry, next) {
			return next
		}
		next = next.Add(1 * time.Minute)
	}

	return from.Add(24 * time.Hour)
}

// MARK: matches
// Checks if a given time matches the cron entry
func (c *CronScheduler) matches(entry *CronEntry, t time.Time) bool {
	minute := t.Minute()
	hour := t.Hour()
	day := t.Day()
	month := int(t.Month())
	weekday := int(t.Weekday())

	return c.contains(entry.Minute, minute) &&
		c.contains(entry.Hour, hour) &&
		c.contains(entry.DayOfMonth, day) &&
		c.contains(entry.Month, month) &&
		c.contains(entry.DayOfWeek, weekday)
}

// MARK: contains
// Helper to check if a value exists in a slice
func (c *CronScheduler) contains(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
