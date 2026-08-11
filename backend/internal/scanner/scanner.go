package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"media-library/backend/internal/applog"
	"media-library/backend/internal/domain"
	"media-library/backend/internal/jobpool"
	"media-library/backend/internal/metadata"
	"media-library/backend/internal/store"
)

type Scanner struct {
	Store    store.Store
	Metadata metadata.Extractor
	Progress func(path string, media bool) error
	// JobID, when set, scopes queued media imports in the shared worker pool
	// so pause/resume/cancel applies to this job.
	JobID string
	// WorkerPool, when set, runs media imports on the shared pool instead of
	// spawning per-scan goroutines. SetCapacity changes made while the job is
	// paused apply automatically to work submitted after the change.
	WorkerPool *jobpool.Pool
	// Paused, when set, reports whether the surrounding job is currently
	// paused. It is consulted at submit time so work queued while the job is
	// paused starts parked (and releases workers to other jobs).
	Paused func() bool
}

func (s Scanner) WithPool(jobID string, pool *jobpool.Pool, paused func() bool) Scanner {
	s.JobID = jobID
	s.WorkerPool = pool
	s.Paused = paused
	return s
}

func (s Scanner) Scan(ctx context.Context, library domain.Library) (err error) {
	if bulk, ok := s.Store.(interface {
		BeginBulk() error
		EndBulk() error
		AbortBulk() error
	}); ok {
		if err := bulk.BeginBulk(); err != nil {
			return err
		}
		defer func() {
			if err != nil {
				if bulkErr := bulk.AbortBulk(); bulkErr != nil {
					err = fmt.Errorf("%w; rollback bulk import: %v", err, bulkErr)
				}
				return
			}
			if bulkErr := bulk.EndBulk(); bulkErr != nil {
				err = bulkErr
			}
		}()
	}
	for _, root := range library.Roots {
		if err := s.scanRoot(ctx, library, root); err != nil {
			return fmt.Errorf("scan root %q: %w", root.Path, err)
		}
	}
	return nil
}

func (s Scanner) CountMediaFiles(ctx context.Context, library domain.Library) (int, error) {
	total := 0
	seen := map[string]bool{}
	for _, rootMapping := range library.Roots {
		root, err := s.resolve(rootMapping.Path)
		if err != nil {
			return 0, err
		}
		if err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			if entry == nil {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if _, ok := s.mimeTypeForPath(ctx, entry.Name()); !ok {
				return nil
			}
			clean := filepath.Clean(filePath)
			if seen[clean] {
				return nil
			}
			seen[clean] = true
			total++
			return nil
		}); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (s Scanner) WithProgress(progress func(path string, media bool) error) Scanner {
	s.Progress = progress
	return s
}

func (s Scanner) scanRoot(ctx context.Context, _ domain.Library, mapping domain.LibraryRoot) error {
	root, err := s.resolve(mapping.Path)
	if err != nil {
		return err
	}
	rootFolder, err := s.Store.UpsertFolder(ctx, domain.MediaFolder{
		ID: mapping.ID, ParentID: domain.InvalidID, Path: root, RelativePath: "",
	})
	if err != nil {
		return err
	}
	rootID := rootFolder.ID
	seenFolders := map[int]bool{rootID: true}
	seenMedia := map[int]bool{}
	var seenMediaMu sync.Mutex
	folderIDs := map[string]int{filepath.Clean(root): rootID}
	type mediaTask struct {
		filePath string
		entry    fs.DirEntry
		mimeType string
		parentID int
	}
	tasks := []mediaTask{}
	if err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.Progress != nil && (walkErr != nil || entry == nil || entry.IsDir()) {
			if err := s.Progress(filePath, false); err != nil {
				return err
			}
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(root, filePath)
			if err != nil {
				return err
			}
			if relative == "." {
				return nil
			}
			relative = filepath.ToSlash(relative)
			parentID, ok := folderIDs[filepath.Clean(filepath.Dir(filePath))]
			if !ok {
				parentID = domain.InvalidID
			}
			folder, err := s.Store.UpsertFolder(ctx, domain.MediaFolder{
				ID: domain.InvalidID, ParentID: parentID, Path: filePath, RelativePath: relative,
			})
			if err != nil {
				return err
			}
			folderIDs[filepath.Clean(filePath)] = folder.ID
			seenFolders[folder.ID] = true
			return nil
		}
		mimeType, ok := s.mimeTypeForPath(ctx, entry.Name())
		if !ok {
			return nil
		}
		tasks = append(tasks, mediaTask{
			filePath: filePath, entry: entry, mimeType: mimeType,
			parentID: folderIDs[filepath.Clean(filepath.Dir(filePath))],
		})
		return nil
	}); err != nil {
		return err
	}
	if len(tasks) > 0 {
		if s.WorkerPool != nil {
			work := make([]jobpool.Work, len(tasks))
			for i, task := range tasks {
				task := task
			work[i] = func(ctx context.Context) error {
				mediaID, err := s.importMedia(ctx, root, task.filePath, task.entry, task.mimeType, task.parentID)
				if err != nil {
					return err
				}
				seenMediaMu.Lock()
				seenMedia[mediaID] = true
				seenMediaMu.Unlock()
				return nil
			}
			}
			paused := s.Paused != nil && s.Paused()
			s.WorkerPool.Submit(s.JobID, ctx, paused, work)
			if err := s.WorkerPool.Wait(ctx, s.JobID); err != nil {
				return err
			}
			return s.Store.PruneFolder(ctx, rootID, seenFolders, seenMedia)
		}
		for _, task := range tasks {
			if err := ctx.Err(); err != nil {
				return err
			}
			mediaID, err := s.importMedia(ctx, root, task.filePath, task.entry, task.mimeType, task.parentID)
			if err != nil {
				return err
			}
			seenMediaMu.Lock()
			seenMedia[mediaID] = true
			seenMediaMu.Unlock()
		}
	}
	return s.Store.PruneFolder(ctx, rootID, seenFolders, seenMedia)
}

func (s Scanner) importMedia(ctx context.Context, root, filePath string, entry fs.DirEntry, mimeType string, parentID int) (int, error) {
	if s.Progress != nil {
		if err := s.Progress(filePath, true); err != nil {
			return domain.InvalidID, err
		}
	}
	info, err := entry.Info()
	if err != nil {
		return domain.InvalidID, err
	}
	relative, err := filepath.Rel(root, filePath)
	if err != nil {
		return domain.InvalidID, err
	}
	relative = filepath.ToSlash(relative)
	// The refresh job only imports new files; rows that already exist are not
	// rewritten or re-extracted. User edits (GPS, name, taken-at) are applied
	// through the dedicated update endpoints, so they are never lost here.
	if existing, existingErr := s.Store.MediaByPath(ctx, filePath); existingErr == nil {
		return existing.ID, nil
	}
	extracted, err := s.metadata().Extract(ctx, filePath, mimeType)
	metadataError := ""
	if err != nil {
		metadataError = err.Error()
	} else {
		metadataError = extracted.Error
	}
	if metadataError != "" {
		applog.Printf(applog.Error, "metadata failed for %s: %s", filePath, metadataError)
	}
	takenAt := extracted.TakenAt
	if takenAt == "" {
		takenAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	media, err := s.Store.UpsertMedia(ctx, domain.Media{
		ID: domain.InvalidID, FolderID: parentID, Path: filePath, RelativePath: relative, Name: entry.Name(),
		Kind: domain.KindFromMIME(mimeType), MIMEType: mimeType, Size: info.Size(),
		Metadata: extracted.Metadata, GPS: extracted.GPS, TakenAt: takenAt, MetadataError: metadataError,
	})
	if err != nil {
		return domain.InvalidID, err
	}
	return media.ID, nil
}

func (s Scanner) mimeTypeForPath(ctx context.Context, path string) (string, bool) {
	mimeType, err := s.Store.MIMETypeForExtension(ctx, filepath.Ext(path))
	return mimeType, err == nil
}

func (s Scanner) MIMETypeForPath(ctx context.Context, path string) (string, bool) {
	return s.mimeTypeForPath(ctx, path)
}

func (s Scanner) metadata() metadata.Extractor {
	if s.Metadata.ExifTool == "" && s.Metadata.FFProbe == "" && s.Metadata.Timeout == 0 {
		return metadata.New()
	}
	return s.Metadata
}

func (s Scanner) resolve(configured string) (string, error) {
	target, err := filepath.Abs(configured)
	if err != nil {
		return "", err
	}
	if evaluated, evalErr := filepath.EvalSymlinks(target); evalErr == nil {
		target = evaluated
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("library path is not a directory")
	}
	return target, nil
}

func (s Scanner) NormalizeRoot(configured string) (string, error) {
	return s.resolve(configured)
}

func NewLibrary(name string, roots []domain.LibraryRoot) domain.Library {
	for index := range roots {
		roots[index].Path = filepath.ToSlash(filepath.Clean(roots[index].Path))
		if roots[index].ID == 0 {
			roots[index].ID = domain.InvalidID
		}
	}
	return domain.Library{ID: domain.InvalidID, Name: name, Roots: roots}
}
