package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"media-library/backend/internal/api"
	"media-library/backend/internal/applog"
	"media-library/backend/internal/domain"
	"media-library/backend/internal/gatewayconfig"
	"media-library/backend/internal/jobpool"
	"media-library/backend/internal/scanner"
	"media-library/backend/internal/scheduler"
	"media-library/backend/internal/store"
	"media-library/backend/internal/transcode"
	"media-library/backend/internal/watcher"
)

// Build-time values injected via -ldflags (see Dockerfile). They are exposed to
// authenticated clients through GET /api/v1/about.
var (
	version   = "dev"
	revision  = "unknown"
	buildDate = "unknown"
)

func main() {
	const addr = ":8080"
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must contain at least 32 characters")
	}

	// Repository: DB_DRIVER selects the backing store. sqlite is the primary
	// implementation, postgres is an alternative. Two dedicated handles are
	// opened so background jobs (scans, metadata renew, thumbnail creation,
	// watcher/scheduler) never share a connection with interactive requests: a
	// wedged or long-running job query cannot starve the UI, and vice versa.
	var repository store.Store
	var jobRepository store.Store
	switch driver := env("DB_DRIVER", "sqlite"); driver {
	case "sqlite":
		sqliteStore, err := store.NewSQLite(env("DB_DSN", "/runtime/app-data/media-library.db"))
		if err != nil {
			log.Fatalf("open sqlite store: %v", err)
		}
		defer sqliteStore.Close()
		repository = sqliteStore
		jobSQLiteStore, err := store.NewSQLite(env("DB_DSN", "/runtime/app-data/media-library.db"))
		if err != nil {
			log.Fatalf("open sqlite job store: %v", err)
		}
		defer jobSQLiteStore.Close()
		jobRepository = jobSQLiteStore
	case "postgres":
		postgresStore, err := store.NewPostgres(env("DB_DSN", "postgres://media:media@localhost:5432/media?sslmode=disable"))
		if err != nil {
			log.Fatalf("open postgres store: %v", err)
		}
		defer postgresStore.Close()
		repository = postgresStore
		jobPostgresStore, err := store.NewPostgres(env("DB_DSN", "postgres://media:media@localhost:5432/media?sslmode=disable"))
		if err != nil {
			log.Fatalf("open postgres job store: %v", err)
		}
		defer jobPostgresStore.Close()
		jobRepository = jobPostgresStore
	default:
		log.Fatalf("unknown DB_DRIVER %q (want sqlite or postgres)", driver)
	}
	poolCapacity := domain.DefaultServerSettings().WorkerPoolSize
	if settings, err := repository.ServerSettings(context.Background()); err == nil {
		if parsed, err := applog.ParseLevel(settings.LogLevel); err == nil {
			applog.SetLevel(parsed)
		}
		if settings.WorkerPoolSize >= 1 {
			poolCapacity = settings.WorkerPoolSize
		}
	}
	workerPool := jobpool.New(64, poolCapacity)
	defer workerPool.Close()
	logPath := env("APP_LOG_FILE", "/runtime/app-config/logs/app.log")
	if err := applog.ConfigureFile(logPath); err != nil {
		log.Fatalf("configure app log: %v", err)
	}
	gatewayPath := env("GATEWAY_CONFIG_FILE", "/runtime/app-config/gateway/Caddyfile")
	if err := gatewayconfig.Write(gatewayPath, gatewayconfig.Load(context.Background(), repository)); err != nil {
		log.Fatalf("write gateway config: %v", err)
	}
	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	apiInstance := &api.API{
		Store:             repository,
		JobStore:          jobRepository,
		Scanner:           scanner.Scanner{Store: jobRepository},
		Transcoder:        transcode.Service{},
		JWTSecret:         []byte(secret),
		GatewayConfigPath: gatewayPath,
		CaddyDataDir:      env("CADDY_DATA_DIR", "/runtime/caddy-data"),
		GatewayEnabled:    envBool("HTTPS_GATEWAY_ENABLED"),
		ThumbnailDir:      env("THUMBNAIL_DIR", "/thumbnails"),
		LogFile:           logPath,
		Shutdown:          stop,
		ContainerStop:     dockerStopSelf,
		WorkerPool:        workerPool,
		Version:           version,
		Revision:          revision,
		BuildDate:         buildDate,
	}
	handler := apiInstance.Handler()

	watcherInstance := watcher.New(jobRepository, apiInstance)
	apiInstance.OnLibrariesChanged = watcherInstance.Refresh
	go watcherInstance.Run(stopCtx)

	go scheduler.Loop(stopCtx, jobRepository, apiInstance)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		applog.Printf(applog.Info, "media API listening on %s", addr)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-stopCtx.Done():
		applog.Printf(applog.Info, "media API shutdown requested")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	applog.Printf(applog.Info, "media API stopped")
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// envBool reports whether a boolean env flag is enabled. The compose file sets
// HTTPS_GATEWAY_ENABLED to "1" when the optional gateway profile is active.
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func dockerStopSelf(ctx context.Context) error {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return fmt.Errorf("docker socket unavailable: %w", err)
	}
	container := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if override := strings.TrimSpace(os.Getenv("DOCKER_SELF_CONTAINER")); override != "" {
		container = override
	}
	if container == "" {
		return errors.New("container id is unavailable")
	}
	output, err := exec.CommandContext(ctx, "docker", "stop", container).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop %s: %w: %s", container, err, strings.TrimSpace(string(output)))
	}
	return nil
}
