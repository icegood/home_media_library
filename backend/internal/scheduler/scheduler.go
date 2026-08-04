package scheduler

import (
	"context"
	"errors"
	"time"

	"media-library/backend/internal/applog"
	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

// Runner starts the jobs that scheduled tasks trigger. It is implemented by the
// API layer so scheduled tasks reuse the existing job system.
type Runner interface {
	StartScan(library domain.Library) error
	StartThumbnails(library domain.Library) error
	StartVacuum() error
}

// Storage is the subset of the store needed by the scheduler loop.
type Storage interface {
	DueScheduledTasks(ctx context.Context, now time.Time) ([]domain.ScheduledTask, error)
	MarkScheduledTaskRun(ctx context.Context, id int, lastRunAt, nextRunAt time.Time) error
	Library(ctx context.Context, id int) (domain.Library, error)
	DisableScheduledTask(ctx context.Context, id int) error
}

// Loop runs every minute, fires due enabled tasks, and reschedules them using
// the stored cron expression. It stops when ctx is cancelled.
func Loop(ctx context.Context, storage Storage, runner Runner) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	// Run once shortly after startup.
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}
	for {
		tick(ctx, storage, runner)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func tick(ctx context.Context, storage Storage, runner Runner) {
	now := time.Now().UTC()
	tasks, err := storage.DueScheduledTasks(ctx, now)
	if err != nil {
		applog.Printf(applog.Error, "scheduled task lookup failed: %v", err)
		return
	}
	for _, task := range tasks {
		if err := fire(ctx, storage, runner, task, now); err != nil {
			applog.Printf(applog.Error, "scheduled task %d (%s) failed: %v", task.ID, task.Name, err)
		}
	}
}

func fire(ctx context.Context, storage Storage, runner Runner, task domain.ScheduledTask, now time.Time) error {
	switch task.TaskType {
	case "scan", "thumbnail-create":
		library, err := storage.Library(ctx, task.LibraryID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				applog.Printf(applog.Warn, "scheduled task %d (%s) references deleted library %d; disabling task", task.ID, task.Name, task.LibraryID)
				return storage.DisableScheduledTask(ctx, task.ID)
			}
			return err
		}
		if task.TaskType == "scan" {
			err = runner.StartScan(library)
		} else {
			err = runner.StartThumbnails(library)
		}
		if err != nil {
			return err
		}
	case "vacuum":
		if err := runner.StartVacuum(); err != nil {
			return err
		}
	default:
		applog.Printf(applog.Warn, "scheduled task %d has unknown type %q", task.ID, task.TaskType)
	}
	next, err := Next(task.Cron, now)
	if err != nil {
		return err
	}
	return storage.MarkScheduledTaskRun(ctx, task.ID, now, next)
}