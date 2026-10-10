package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearFileTruncatesThroughWriterHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := ConfigureFile(path); err != nil {
		t.Fatal(err)
	}
	Printf(Info, "first line")
	Printf(Info, "second line")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(string(data)), "\n")) != 2 {
		t.Fatalf("expected two log lines before clear, got: %q", string(data))
	}
	if err := ClearFile(); err != nil {
		t.Fatal(err)
	}
	Printf(Info, "fresh line")
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "first line") || strings.Contains(string(data), "second line") {
		t.Fatalf("log file still contains old lines after clear: %q", string(data))
	}
	if !strings.Contains(string(data), "fresh line") {
		t.Fatalf("log file missing the line written after clear: %q", string(data))
	}
}

func TestClearFileWithoutConfiguredFile(t *testing.T) {
	fileMu.Lock()
	saved := logFile
	logFile = nil
	fileMu.Unlock()
	t.Cleanup(func() {
		fileMu.Lock()
		logFile = saved
		fileMu.Unlock()
	})
	if err := ClearFile(); err != ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got: %v", err)
	}
}

func TestFileRotatesBySize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := ConfigureFile(path); err != nil {
		t.Fatal(err)
	}
	SetRotation(Rotation{MaxSizeMB: 1, MaxBackups: 2, MaxAgeDays: 30})
	t.Cleanup(func() { SetRotation(Rotation{}) })
	line := strings.Repeat("x", 64*1024)
	for i := 0; i < 40; i++ {
		Printf(Error, "%s", line)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatalf("active log is %d bytes, want <= 1 MiB", info.Size())
	}
	for _, index := range []string{".1", ".2"} {
		if _, err := os.Stat(path + index); err != nil {
			t.Fatalf("backup %s missing: %v", index, err)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Fatal("backup beyond MaxBackups was kept")
	}
}
