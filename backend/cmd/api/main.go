// Command api serves the ttacommunity JSON API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/discord"
	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/httpapi"
	"ttacommunity/backend/internal/metrics"
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
	reg := metrics.New()
	reg.AddPool("ratings_read", pool)
	ratings := store.NewRatingsReader(pool)
	if err := ratings.CheckReadOnly(ctx); err != nil {
		log.Error("refusing to start: the ratings read role must be read-only", "err", err)
		os.Exit(1)
	}

	admins, adminPool, err := openAdmin(ctx, addr)
	if err != nil {
		log.Error("refusing to start: admin is not set up", "err", err)
		os.Exit(1)
	}
	defer adminPool.Close()
	reg.AddPool("ratings_admin", adminPool)
	if os.Getenv("ADMIN_DEV_USER") != "" {
		log.Warn("admin sign-in is bypassed for local development (ADMIN_DEV_USER)")
	}

	signIns, playerPool, err := openSignIns(ctx, os.Getenv)
	if err != nil {
		log.Error("refusing to start: player sign-in is not set up", "err", err)
		os.Exit(1)
	}
	defer playerPool.Close()
	reg.AddPool("ratings_player", playerPool)
	if os.Getenv("DISCORD_DEV_USER") != "" {
		log.Warn("Discord sign-in is a stand-in for local development (DISCORD_DEV_USER)")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("listen failed", "addr", addr, "err", err)
		os.Exit(1)
	}
	log.Info("api listening", "addr", ln.Addr().String())
	serveMetrics(ctx, os.Getenv("METRICS_ADDR"), reg, log)
	if err := httpapi.Serve(ctx, ln, httpapi.NewMux(ratings, admins, signIns, log, reg, time.Now), log); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
	log.Info("api stopped cleanly")
}

// serveMetrics serves reg at GET /metrics on addr, a listener of its own that the tunnel never reaches,
// until ctx ends. With addr empty, metrics are kept but not served.
func serveMetrics(ctx context.Context, addr string, reg *metrics.Registry, log *slog.Logger) {
	if addr == "" {
		log.Info("metrics are not served: METRICS_ADDR is not set")
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("metrics listen failed", "addr", addr, "err", err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", reg.Handler())
	log.Info("metrics listening", "addr", ln.Addr().String())
	go func() {
		if err := httpapi.Serve(ctx, ln, mux, log); err != nil {
			log.Error("metrics stopped", "err", err)
		}
	}()
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
// It returns admin and its pool, for the caller to close and report on.
func openAdmin(ctx context.Context, addr string) (adminDeps, *pgxpool.Pool, error) {
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
	return adminDeps{v, a, origin, starter}, pool, nil
}

// discordSignIn starts and finishes sign-ins with Discord, or with devDiscord standing in for it.
type discordSignIn interface {
	AuthCodeURL(state, verifier string) string
	User(ctx context.Context, code, verifier string) (discord.User, error)
}

// signInDeps signs players in with Discord, keeps their sessions through the player login, and knows the site.
type signInDeps struct {
	*store.PlayerStore
	discord discordSignIn
	site    string
}

// SiteOrigin is the site's origin: the only one players' requests may come from, and where sign-in returns.
func (d signInDeps) SiteOrigin() string { return d.site }

// DiscordAuthURL is where a sign-in sends the browser to Discord.
func (d signInDeps) DiscordAuthURL(state, verifier string) string {
	return d.discord.AuthCodeURL(state, verifier)
}

// DiscordUser finishes a sign-in with the code Discord sent back.
func (d signInDeps) DiscordUser(ctx context.Context, code, verifier string) (discord.User, error) {
	return d.discord.User(ctx, code, verifier)
}

// devDiscord stands in for Discord in local development: the browser goes straight back to the callback, and
// every sign-in is the invented account named username, with an id made from that name.
type devDiscord struct {
	api, username string
}

func (d devDiscord) AuthCodeURL(state, _ string) string {
	return d.api + "/v1/auth/discord/callback?code=dev&state=" + url.QueryEscape(state)
}

func (d devDiscord) User(_ context.Context, code, _ string) (discord.User, error) {
	if code != "dev" {
		return discord.User{}, errors.New("the Discord stand-in sent no such code")
	}
	h := fnv.New64a()
	h.Write([]byte(d.username))
	return discord.User{ID: fmt.Sprintf("1%017d", h.Sum64()%1e17), Username: d.username}, nil
}

// openSignIns builds player sign-in from getenv's SITE_ORIGIN, API_ORIGIN (this api's public origin, for Discord's
// callback), RATINGS_PLAYER_DATABASE_URL and DISCORD_CLIENT_ID and _SECRET. DISCORD_DEV_USER, instead of the
// Discord app, signs everyone in as that invented account, and only on this machine: API_ADDR and SITE_ORIGIN on
// 127.0.0.1, outside Kubernetes. Anything missing or wrong, or a player login that can change more than sign-in
// needs, is an error: sign-in never starts open. It returns sign-in and its pool, for the caller to close.
func openSignIns(ctx context.Context, getenv func(string) string) (signInDeps, *pgxpool.Pool, error) {
	site, api := getenv("SITE_ORIGIN"), getenv("API_ORIGIN")
	if !isOrigin(site) {
		return signInDeps{}, nil, errors.New("SITE_ORIGIN must be the site's origin: https://host, or http://127.0.0.1:port")
	}
	if !isOrigin(api) {
		return signInDeps{}, nil, errors.New("API_ORIGIN must be this api's public origin: https://host, or http://127.0.0.1:port")
	}
	dbURL := getenv("RATINGS_PLAYER_DATABASE_URL")
	if dbURL == "" {
		return signInDeps{}, nil, errors.New("RATINGS_PLAYER_DATABASE_URL is not set")
	}
	var dc discordSignIn
	if dev := getenv("DISCORD_DEV_USER"); dev != "" {
		if !strings.HasPrefix(getenv("API_ADDR"), "127.0.0.1:") || getenv("KUBERNETES_SERVICE_HOST") != "" ||
			!strings.HasPrefix(site, "http://127.0.0.1:") || getenv("DISCORD_CLIENT_ID") != "" {
			return signInDeps{}, nil, errors.New("DISCORD_DEV_USER is for this machine alone: API_ADDR and SITE_ORIGIN on 127.0.0.1, " +
				"outside Kubernetes, and no Discord app")
		}
		dc = devDiscord{api: api, username: dev}
	} else {
		c, err := discord.New(discord.Config{
			ClientID:     getenv("DISCORD_CLIENT_ID"),
			ClientSecret: getenv("DISCORD_CLIENT_SECRET"),
			RedirectURL:  api + "/v1/auth/discord/callback",
		}, &http.Client{Timeout: 10 * time.Second})
		if err != nil {
			return signInDeps{}, nil, fmt.Errorf("DISCORD_CLIENT_ID and _SECRET: %w", err)
		}
		dc = c
	}
	pool, err := store.OpenPool(ctx, dbURL)
	if err != nil {
		return signInDeps{}, nil, errors.New("opening the player pool failed")
	}
	p := store.NewPlayerStore(pool)
	if err := p.CheckLimits(ctx); err != nil {
		pool.Close()
		return signInDeps{}, nil, err
	}
	return signInDeps{PlayerStore: p, discord: dc, site: site}, pool, nil
}

// isOrigin reports whether s is a bare https origin, or http on 127.0.0.1 for local development: no path.
func isOrigin(s string) bool {
	rest, ok := strings.CutPrefix(s, "https://")
	if !ok {
		rest, ok = strings.CutPrefix(s, "http://127.0.0.1:")
	}
	return ok && rest != "" && !strings.ContainsAny(rest, "/?#@")
}
