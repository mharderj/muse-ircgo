// Command ircgo is a terminal IRC client built on Bubble Tea.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"ircgo/internal/config"
	"ircgo/internal/history"
	"ircgo/internal/irc"
	"ircgo/internal/store"
	"ircgo/internal/ui"
	"ircgo/internal/version"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	debug := flag.Bool("debug", false, "write connection diagnostics to debug.log (passwords redacted)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ircgo v%s\n", version.Version)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ircgo: %v\n", err)
		os.Exit(1)
	}

	// Channel logging defaults to ~/.local/ircgo/logs. `logging = false`
	// disables it entirely; `log_dir` points it elsewhere.
	if !cfg.LoggingOn() {
		history.LogDir = ""
	} else if dir := cfg.ExpandedLogDir(); dir != "" {
		history.LogDir = dir
	}

	if *debug {
		logDir := history.LocalDir()
		if logDir == "" {
			logDir = filepath.Dir(*cfgPath)
		}
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "ircgo: cannot create debug log dir: %v\n", err)
			os.Exit(1)
		}
		logPath := filepath.Join(logDir, "debug.log")
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ircgo: cannot open debug log: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		log.SetOutput(f)
		irc.Debug = true
		fmt.Fprintf(os.Stderr, "ircgo: debug logging to %s\n", logPath)
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

	app := ui.New(cfg, st, events, clients)
	app.ConfigPath = *cfgPath
	p := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(), // hover x on sidebar buffers + click to switch
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ircgo: %v\n", err)
		os.Exit(1)
	}
	stop()
}
