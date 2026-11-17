// Command discord-commands registers the Discord bot's slash commands everywhere the app is installed, or in one
// server with -server; -clear removes that server's own commands. The bot token comes from DISCORD_BOT_TOKEN, never
// a flag, so no shell history holds it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"ttacommunity/backend/internal/discordcommands"
)

func main() {
	app := flag.String("app", "", "the Discord application's id")
	server := flag.String("server", "", "a Discord server's id, to register there alone instead of everywhere")
	clear := flag.Bool("clear", false, "remove the server's own commands, leaving those registered everywhere")
	flag.Parse()
	commands := discordcommands.Commands
	if *clear {
		if *server == "" {
			fmt.Fprintln(os.Stderr, errors.New("-clear needs -server"))
			os.Exit(2)
		}
		commands = []byte("[]")
	}
	names, err := discordcommands.Register(context.Background(), &http.Client{Timeout: 30 * time.Second}, discordcommands.API,
		*app, *server, os.Getenv("DISCORD_BOT_TOKEN"), commands)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	where := "everywhere"
	if *server != "" {
		where = "in " + *server
	}
	fmt.Printf("registered %s: %s\n", where, strings.Join(names, ", "))
}
