package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

type stubStorage struct {
	libraryErr error
	disabled   bool
	marked     bool
}

func (s *stubStorage) DueScheduledTasks(context.Context, time.Time) ([]domain.ScheduledTask, error) {
	return nil, nil
}

func (s *stubStorage) MarkScheduledTaskRun(context.Context, int, time.Time, time.Time) error {
	s.marked = true
	return nil
}

func (s *stubStorage) Library(context.Context, int) (domain.Library, error) {
	return domain.Library{}, s.libraryErr
}

func (s *stubStorage) DisableScheduledTask(context.Context, int) error {
	s.disabled = true
	return nil
}

type stubRunner struct {
	scans    int
	startErr error
}

func (r *stubRunner) StartScan(domain.Library) error {
	r.scans++
	return r.startErr
}

func (r *stubRunner) StartThumbnails(domain.Library) error { return nil }

func (r *stubRunner) StartVacuum() error { return nil }

func TestFireDisablesTaskWhenLibraryMissing(t *testing.T) {
	storage := &stubStorage{libraryErr: store.ErrNotFound}
	task := domain.ScheduledTask{ID: 7, Name: "scan", TaskType: "scan", LibraryID: 3, Cron: "0 3 * * *"}
	if err := fire(context.Background(), storage, &stubRunner{}, task, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if !storage.disabled {
		t.Fatal("task should be disabled when its library is gone")
	}
	if storage.marked {
		t.Fatal("missing-library task must not be marked as run")
	}
}

func TestFireDisablesTaskOnInvalidCron(t *testing.T) {
	storage := &stubStorage{}
	runner := &stubRunner{}
	task := domain.ScheduledTask{ID: 10, Name: "broken", TaskType: "scan", LibraryID: 1, Cron: "not-a-cron"}
	if err := fire(context.Background(), storage, runner, task, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if runner.scans != 0 {
		t.Fatalf("job must not start on an invalid cron, started %d", runner.scans)
	}
	if !storage.disabled {
		t.Fatal("task with invalid cron should be disabled")
	}
	if storage.marked {
		t.Fatal("task with invalid cron must not be marked as run")
	}
}

func TestFireMarksRunEvenWhenStartFails(t *testing.T) {
	storage := &stubStorage{}
	runner := &stubRunner{startErr: errors.New("boom")}
	task := domain.ScheduledTask{ID: 11, Name: "scan", TaskType: "scan", LibraryID: 1, Cron: "0 3 * * *"}
	if err := fire(context.Background(), storage, runner, task, time.Now().UTC()); err != nil {
		t.Fatalf("start failure must not fail the tick: %v", err)
	}
	if runner.scans != 1 {
		t.Fatalf("scan should have been attempted once, got %d", runner.scans)
	}
	if !storage.marked {
		t.Fatal("task must be marked as run even when the start fails")
	}
}

func TestFireStartsScanAndMarksRun(t *testing.T) {
	storage := &stubStorage{}
	runner := &stubRunner{}
	task := domain.ScheduledTask{ID: 8, Name: "scan", TaskType: "scan", LibraryID: 1, Cron: "0 3 * * *"}
	if err := fire(context.Background(), storage, runner, task, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if runner.scans != 1 {
		t.Fatalf("scan should have been started once, got %d", runner.scans)
	}
	if !storage.marked {
		t.Fatal("task should be marked as run")
	}
}

func TestFireMarksRunForVacuum(t *testing.T) {
	storage := &stubStorage{}
	task := domain.ScheduledTask{ID: 9, Name: "vacuum", TaskType: "vacuum", Cron: "0 4 * * *"}
	if err := fire(context.Background(), storage, &stubRunner{}, task, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if !storage.marked {
		t.Fatal("vacuum task should be marked as run")
	}
}
