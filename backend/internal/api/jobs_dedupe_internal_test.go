package api

import (
	"testing"

	"media-library/backend/internal/domain"
)

// The duplicate-job guard lives in the start* wrappers; exercising it directly
// avoids racing a real scan (which finishes too fast on fixtures).
func TestStartScanJobReturnsActiveJobForSameLibrary(t *testing.T) {
	a := &API{jobs: map[string]*JobStatus{}}
	a.jobs["j1"] = &JobStatus{ID: "j1", Category: "scan", LibraryID: 7, Status: "running"}
	got := a.startScanJob(domain.Library{ID: 7, Name: "Same"})
	if got.ID != "j1" {
		t.Fatalf("startScanJob started %q, want active job j1", got.ID)
	}
	a.jobs["j1"].Status = "paused"
	got = a.startScanJob(domain.Library{ID: 7, Name: "Same"})
	if got.ID != "j1" {
		t.Fatalf("startScanJob started %q while j1 paused, want j1", got.ID)
	}
}

func TestStartVacuumJobReturnsActiveVacuum(t *testing.T) {
	a := &API{jobs: map[string]*JobStatus{}}
	a.jobs["v1"] = &JobStatus{ID: "v1", Category: "vacuum", LibraryID: 0, Status: "running"}
	got, ok := a.startVacuumJob()
	if !ok {
		t.Fatal("startVacuumJob reported unsupported")
	}
	if got.ID != "v1" {
		t.Fatalf("startVacuumJob started %q, want active vacuum v1", got.ID)
	}
}

func TestApplyRenewResume(t *testing.T) {
	items := []domain.Media{{ID: 1}, {ID: 2}, {ID: 3}}
	if got := applyRenewResume(items, 0); len(got) != 3 {
		t.Fatalf("zero processed must keep all items, got %d", len(got))
	}
	if got := applyRenewResume(items, 2); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("resume must skip first two items, got %#v", got)
	}
	if got := applyRenewResume(items, 5); got != nil {
		t.Fatalf("processed beyond length must yield empty list, got %#v", got)
	}
}
