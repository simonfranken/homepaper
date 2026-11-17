// Command homepaper runs the server: HA connection, layout editor, renderer
// and device WebSocket.
package main

import (
	"context"
	"errors"
	"fmt"
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
// working directory). Variables already set in the environment win.
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

	// One global Home Assistant connection from the environment (D-016).
	if haURL != "" && haToken == "" {
		return errors.New("HA_URL is set but HA_TOKEN is empty")
	}
	if haURL != "" {
		if haURL, err = ha.NormalizeURL(haURL); err != nil {
			return fmt.Errorf("HA_URL: %w", err)
		}
	}
	if old := st.HA(); old.URL != "" {
		log.Warn("homepaper.json still holds a Home Assistant connection saved in the web UI; it is no longer used. Set HA_URL and HA_TOKEN instead.", "saved_url", old.URL)
	}
	if os.Getenv("HOMEPAPER_PASSWORD") != "" {
		log.Warn("HOMEPAPER_PASSWORD is no longer used: the web UI logs in through Home Assistant")
	}

	// Login via Home Assistant (D-016, D-017).
	var auth *api.Auth
	switch strings.ToLower(env("HOMEPAPER_AUTH", "on")) {
	case "off":
		log.Warn("HOMEPAPER_AUTH=off: the web UI and API are open to everyone who can reach them. Use this only for development or the demo.")
	case "on":
		if haURL == "" {
			return errors.New("HA_URL and HA_TOKEN are required: homepaper logs users in through Home Assistant. For a demo without Home Assistant, set HOMEPAPER_AUTH=off")
		}
		key, err := api.SessionKey(dataDir)
		if err != nil {
			return fmt.Errorf("session key: %w", err)
		}
		publicHA := haURL
		if v := os.Getenv("HA_PUBLIC_URL"); v != "" {
			if publicHA, err = ha.NormalizeURL(v); err != nil {
				return fmt.Errorf("HA_PUBLIC_URL: %w", err)
			}
		}
		login := strings.ToLower(env("HOMEPAPER_LOGIN", "admins"))
		if login != "admins" && login != "users" {
			return fmt.Errorf("HOMEPAPER_LOGIN must be admins or users, not %q", login)
		}
		auth = &api.Auth{HAURL: haURL, PublicHAURL: publicHA, PublicURL: os.Getenv("HOMEPAPER_URL"),
			AdminsOnly: login == "admins", Key: key, HTTP: &http.Client{Timeout: 15 * time.Second}}
		log.Info("login via home assistant", "who", login, "ha", publicHA)
	default:
		return errors.New("HOMEPAPER_AUTH must be on or off")
	}
	manager := ha.NewManager(ctx, log)
	states := manager.Store

	settings := devicehub.DefaultSettings()
	if v, err := strconv.Atoi(os.Getenv("HOMEPAPER_FULL_REFRESH_EVERY")); err == nil && v > 0 {
		settings.FullRefreshEvery = v
	}
	if v, err := time.ParseDuration(os.Getenv("HOMEPAPER_MIN_INTERVAL")); err == nil {
		settings.MinInterval = v
	}
	hub := devicehub.New(st, states, log, settings)

	a := &api.API{Store: st, States: states, Hub: hub, Log: log, Web: web.Dist(), Location: time.Local,
		HA: manager, HAURL: haURL, HAToken: haToken, Auth: auth}
	a.StartHA()
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
