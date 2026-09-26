// Command api serves the ttacommunity JSON API.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"ttacommunity/backend/internal/httpapi"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error("listen failed", "addr", addr, "err", err)
		os.Exit(1)
	}
	log.Info("api listening", "addr", ln.Addr().String())
	if err := httpapi.Serve(ctx, ln, httpapi.NewMux()); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
	log.Info("api stopped cleanly")
}
