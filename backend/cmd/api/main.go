// Command api serves the ttacommunity JSON API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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

	pool, err := pgxpool.New(ctx, readURL)
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

	admins, closeAdmins, err := openAdmin(ctx)
	if err != nil {
		log.Error("refusing to start: admin is not set up", "err", err)
		os.Exit(1)
	}
	defer closeAdmins()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("listen failed", "addr", addr, "err", err)
		os.Exit(1)
	}
	log.Info("api listening", "addr", ln.Addr().String())
	if err := httpapi.Serve(ctx, ln, httpapi.NewMux(ratings, admins, log)); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
	log.Info("api stopped cleanly")
}

// adminDeps verifies admin requests' Access tokens and reads admins through the admin login.
type adminDeps struct {
	*access.Verifier
	*store.AdminStore
}

// openAdmin builds admin from ACCESS_TEAM, ACCESS_AUD and RATINGS_ADMIN_DATABASE_URL. Any of them
// unset, or an admin login that can change more than admin edits, is an error: admin never starts open.
func openAdmin(ctx context.Context) (adminDeps, func(), error) {
	team, aud, url := os.Getenv("ACCESS_TEAM"), os.Getenv("ACCESS_AUD"), os.Getenv("RATINGS_ADMIN_DATABASE_URL")
	if url == "" {
		return adminDeps{}, nil, errors.New("RATINGS_ADMIN_DATABASE_URL is not set")
	}
	v, err := access.NewVerifier(access.Config{Team: team, AUD: aud}, &http.Client{Timeout: 10 * time.Second}, time.Now)
	if err != nil {
		return adminDeps{}, nil, err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return adminDeps{}, nil, errors.New("opening the admin pool failed")
	}
	a := store.NewAdminStore(pool)
	if err := a.CheckLimits(ctx); err != nil {
		pool.Close()
		return adminDeps{}, nil, err
	}
	return adminDeps{v, a}, pool.Close, nil
}
