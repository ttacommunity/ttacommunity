// Command api serves the ttacommunity JSON API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/store"
)

// migrate applies the ratings migrations as url's login, the database's owner.
func migrate(ctx context.Context, url string) error {
	if url == "" {
		return errors.New("RATINGS_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("opening the ratings database: %w", err)
	}
	defer pool.Close()
	return store.MigrateRatings(ctx, pool)
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	// The api pod's init container runs -migrate first, so a deploy applies its own migrations.
	migrateOnly := flag.Bool("migrate", false, "apply the ratings migrations as RATINGS_DATABASE_URL's login, then exit")
	flag.Parse()
	if *migrateOnly {
		if err := migrate(context.Background(), os.Getenv("RATINGS_DATABASE_URL")); err != nil {
			log.Error("migrating the ratings database failed", "err", err)
			os.Exit(1)
		}
		log.Info("ratings migrations applied")
		return
	}
	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	readURL := os.Getenv("RATINGS_READ_DATABASE_URL")
	if readURL == "" {
		log.Error("RATINGS_READ_DATABASE_URL is not set: refusing to start")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.OpenPool(ctx, readURL)
	if err != nil {
		log.Error("opening the ratings read pool failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	ratings := store.NewRatingsReader(pool)
	if err := ratings.CheckReadOnly(ctx); err != nil {
		log.Error("refusing to start: the ratings read role must be read-only", "err", err)
		os.Exit(1)
	}

	admins, closeAdmins, err := openAdmin(ctx, addr)
	if err != nil {
		log.Error("refusing to start: admin is not set up", "err", err)
		os.Exit(1)
	}
	defer closeAdmins()
	if os.Getenv("ADMIN_DEV_USER") != "" {
		log.Warn("admin sign-in is bypassed for local development (ADMIN_DEV_USER)")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("listen failed", "addr", addr, "err", err)
		os.Exit(1)
	}
	log.Info("api listening", "addr", ln.Addr().String())
	if err := httpapi.Serve(ctx, ln, httpapi.NewMux(ratings, admins, log), log); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
	log.Info("api stopped cleanly")
}

// adminDeps verifies admin requests' Access tokens, reads admins through the admin login, and starts runs.
type adminDeps struct {
	*access.Verifier
	*store.AdminStore
	origin  string
	starter *enginejob.Starter
}

// AllowedOrigin is the admin site's origin, the only one a change may come from.
func (d adminDeps) AllowedOrigin() string { return d.origin }

// StartRun starts the engine now, or returns enginejob.ErrNotSetUp when ENGINE_CRONJOB is not set.
func (d adminDeps) StartRun(ctx context.Context) (string, error) {
	if d.starter == nil {
		return "", enginejob.ErrNotSetUp
	}
	return d.starter.Start(ctx)
}

// StopRun stops the engine's unfinished Jobs, or returns enginejob.ErrNotSetUp when ENGINE_CRONJOB is not set.
func (d adminDeps) StopRun(ctx context.Context) ([]string, error) {
	if d.starter == nil {
		return nil, enginejob.ErrNotSetUp
	}
	return d.starter.Stop(ctx)
}

// EngineGoing reports whether an engine Job is unfinished, or returns enginejob.ErrNotSetUp when ENGINE_CRONJOB is not set.
func (d adminDeps) EngineGoing(ctx context.Context) (bool, error) {
	if d.starter == nil {
		return false, enginejob.ErrNotSetUp
	}
	return d.starter.Going(ctx)
}

// openAdmin builds admin from ACCESS_TEAM, ACCESS_AUD, ADMIN_ORIGIN and RATINGS_ADMIN_DATABASE_URL. Any of them
// unset, or an admin login that can change more than admin edits, is an error: admin never starts open.
// ENGINE_CRONJOB, when set, names the CronJob admin starts runs from, in the pod's namespace.
// ADMIN_DEV_USER, when set, bypasses Access for local development; addr must then be 127.0.0.1.
func openAdmin(ctx context.Context, addr string) (adminDeps, func(), error) {
	team, aud, url := os.Getenv("ACCESS_TEAM"), os.Getenv("ACCESS_AUD"), os.Getenv("RATINGS_ADMIN_DATABASE_URL")
	if url == "" {
		return adminDeps{}, nil, errors.New("RATINGS_ADMIN_DATABASE_URL is not set")
	}
	origin := os.Getenv("ADMIN_ORIGIN")
	if !strings.HasPrefix(origin, "https://") && !strings.HasPrefix(origin, "http://127.0.0.1:") {
		return adminDeps{}, nil, errors.New("ADMIN_ORIGIN must be the admin site's https origin")
	}
	devUser := os.Getenv("ADMIN_DEV_USER")
	v, err := access.NewVerifier(access.Config{
		Team:         team,
		AUD:          aud,
		DevUser:      devUser,
		Origin:       origin,
		ListenAddr:   addr,
		InKubernetes: os.Getenv("KUBERNETES_SERVICE_HOST") != "",
	}, &http.Client{Timeout: 10 * time.Second}, time.Now)
	if err != nil {
		return adminDeps{}, nil, err
	}
	pool, err := store.OpenPool(ctx, url)
	if err != nil {
		return adminDeps{}, nil, errors.New("opening the admin pool failed")
	}
	a := store.NewAdminStore(pool)
	if err := a.CheckLimits(ctx); err != nil {
		pool.Close()
		return adminDeps{}, nil, err
	}
	var starter *enginejob.Starter
	if cj := os.Getenv("ENGINE_CRONJOB"); cj != "" {
		if starter, err = enginejob.InCluster(cj); err != nil {
			pool.Close()
			return adminDeps{}, nil, err
		}
	}
	return adminDeps{v, a, origin, starter}, pool.Close, nil
}
