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
func NewCronScheduler(logger *internal.Logger) *CronScheduler {
	return &CronScheduler{logger: logger}
}

// MARK: Start
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
func (c *CronScheduler) NextRun() time.Time {
	return c.nextRun
}

// MARK: UpdateSchedule
func (c *CronScheduler) UpdateSchedule(schedule string) error {
	wasRunning := atomic.LoadInt64(&c.running) == 1
	if wasRunning {
		c.Stop()
		return c.Start(schedule, c.taskFunc)
	}
	c.schedule = schedule
	return nil
}

// MARK: run
func (c *CronScheduler) run(entry *CronEntry) {
	ticker := time.NewTicker(time.Minute)
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
				c.logger.Debug("Next run scheduled", "time", c.nextRun)
			}
		}
	}
}

// MARK: parseSchedule
func (c *CronScheduler) parseSchedule(schedule string) (*CronEntry, error) {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron format: got %d fields", len(fields))
	}

	entry := &CronEntry{}
	var err error

	if entry.Minute, err = c.parseField(fields[0], 0, 59); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if entry.Hour, err = c.parseField(fields[1], 0, 23); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if entry.DayOfMonth, err = c.parseField(fields[2], 1, 31); err != nil {
		return nil, fmt.Errorf("day of month: %w", err)
	}
	if entry.Month, err = c.parseField(fields[3], 1, 12); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	if entry.DayOfWeek, err = c.parseField(fields[4], 0, 6); err != nil {
		return nil, fmt.Errorf("day of week: %w", err)
	}
	return entry, nil
}

// MARK: parseField
func (c *CronScheduler) parseField(field string, min, max int) ([]int, error) {
	if field == "*" {
		all := make([]int, max-min+1)
		for i := range all {
			all[i] = min + i
		}
		return all, nil
	}

	var result []int
	for _, part := range strings.Split(field, ",") {
		switch {
		case strings.Contains(part, "/"):
			stepParts := strings.Split(part, "/")
			if len(stepParts) != 2 {
				return nil, fmt.Errorf("invalid step format: %s", part)
			}
			step, err := strconv.Atoi(stepParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid step value: %s", stepParts[1])
			}
			rangePart := stepParts[0]
			switch {
			case rangePart == "*":
				for i := min; i <= max; i += step {
					result = append(result, i)
				}
			case strings.Contains(rangePart, "-"):
				rangeValues, err := c.parseRange(rangePart, min, max)
				if err != nil {
					return nil, err
				}
				for i := rangeValues[0]; i <= rangeValues[len(rangeValues)-1]; i += step {
					result = append(result, i)
				}
			default:
				start, err := strconv.Atoi(rangePart)
				if err != nil {
					return nil, fmt.Errorf("invalid start: %s", rangePart)
				}
				if start < min || start > max {
					return nil, fmt.Errorf("value %d out of range [%d,%d]", start, min, max)
				}
				for i := start; i <= max; i += step {
					result = append(result, i)
				}
			}
		case strings.Contains(part, "-"):
			rangeValues, err := c.parseRange(part, min, max)
			if err != nil {
				return nil, err
			}
			result = append(result, rangeValues...)
		default:
			value, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			if value < min || value > max {
				return nil, fmt.Errorf("value %d out of range [%d,%d]", value, min, max)
			}
			result = append(result, value)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid values from: %s", field)
	}
	return result, nil
}

// MARK: parseRange
func (c *CronScheduler) parseRange(rangeStr string, min, max int) ([]int, error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range: %s", rangeStr)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid start: %s", parts[0])
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid end: %s", parts[1])
	}
	if start < min || end > max || start > end {
		return nil, fmt.Errorf("range [%d,%d] out of bounds [%d,%d]", start, end, min, max)
	}

	out := make([]int, end-start+1)
	for i := range out {
		out[i] = start + i
	}
	return out, nil
}

// MARK: calculateNextRun
func (c *CronScheduler) calculateNextRun(entry *CronEntry, from time.Time) time.Time {
	next := from.Add(time.Minute).Truncate(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if c.matches(entry, next) {
			return next
		}
		next = next.Add(time.Minute)
	}
	return from.Add(24 * time.Hour)
}

// MARK: matches
func (c *CronScheduler) matches(entry *CronEntry, t time.Time) bool {
	return c.contains(entry.Minute, t.Minute()) &&
		c.contains(entry.Hour, t.Hour()) &&
		c.contains(entry.DayOfMonth, t.Day()) &&
		c.contains(entry.Month, int(t.Month())) &&
		c.contains(entry.DayOfWeek, int(t.Weekday()))
}

// MARK: contains
func (c *CronScheduler) contains(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
