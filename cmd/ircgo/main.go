// Command ircgo is a terminal IRC client built on Bubble Tea.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/irc"
	"ircgo/internal/store"
	"ircgo/internal/ui"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ircgo: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st := store.New()
	events := make(chan irc.Event, 256)

	clients := make(map[string]*irc.Client, len(cfg.Servers))
	for _, srv := range cfg.Servers {
		cl := irc.New(srv, events)
		clients[srv.Name] = cl
		go cl.Run(ctx)
	}

	p := tea.NewProgram(ui.New(cfg, st, events, clients), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ircgo: %v\n", err)
		os.Exit(1)
	}
	stop()
}
