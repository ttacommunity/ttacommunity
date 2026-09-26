// Command worker runs the timed jobs: reminders, kick-offs and blitz rounds.
// It has no jobs yet, and waits for a stop signal.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("worker started")
	<-ctx.Done()
	log.Info("worker stopped cleanly")
}
