// Command homepaper runs the server: HA connection, layout editor, renderer
// and device WebSocket.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"homepaper/internal/api"
	"homepaper/internal/devicehub"
	"homepaper/internal/ha"
	"homepaper/internal/store"
	"homepaper/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadDotEnv sets variables from a KEY=VALUE file (default .env in the
// working directory). Variables already set in the environment win. This is
// the interim way to configure HA until it moves into the UI (D-013).
func loadDotEnv(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, set := os.LookupEnv(key); !set && val != "" {
			os.Setenv(key, val)
		}
	}
	return true, nil
}

func main() {
	envFile := env("HOMEPAPER_ENV_FILE", ".env")
	loaded, envErr := loadDotEnv(envFile)

	level := slog.LevelInfo
	if os.Getenv("HOMEPAPER_DEBUG") != "" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	if envErr != nil {
		log.Error("cannot read env file", "file", envFile, "err", envErr)
		os.Exit(1)
	}
	if loaded {
		log.Info("loaded settings", "file", envFile)
	}
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listen := env("HOMEPAPER_LISTEN", ":8080")
	dataDir := env("HOMEPAPER_DATA", "data")
	haURL, haToken := os.Getenv("HA_URL"), os.Getenv("HA_TOKEN")

	st, err := store.Open(filepath.Join(dataDir, "homepaper.json"))
	if err != nil {
		return err
	}

	var states *ha.Store
	if haURL == "" {
		log.Warn("HA_URL not set: running with demo entities")
		states = ha.NewStore("demo")
		go ha.RunDemo(ctx, states)
	} else {
		if haToken == "" {
			return errors.New("HA_URL is set but HA_TOKEN is empty")
		}
		states = ha.NewStore("homeassistant")
		go (&ha.Client{URL: haURL, Token: haToken, Store: states, Log: log}).Run(ctx)
	}

	settings := devicehub.DefaultSettings()
	if v, err := strconv.Atoi(os.Getenv("HOMEPAPER_FULL_REFRESH_EVERY")); err == nil && v > 0 {
		settings.FullRefreshEvery = v
	}
	if v, err := time.ParseDuration(os.Getenv("HOMEPAPER_MIN_INTERVAL")); err == nil {
		settings.MinInterval = v
	}
	hub := devicehub.New(st, states, log, settings)

	a := &api.API{Store: st, States: states, Hub: hub, Log: log, Web: web.Dist(),
		Password: os.Getenv("HOMEPAPER_PASSWORD"), Location: time.Local}
	srv := &http.Server{Addr: listen, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("homepaper listening", "addr", listen, "data", dataDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
