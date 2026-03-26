package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sithuaung/gocket/internal/api"
	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/channel"
	"github.com/sithuaung/gocket/internal/server"
	"github.com/sithuaung/gocket/internal/state"
	"github.com/sithuaung/gocket/internal/tui"
	"github.com/sithuaung/gocket/internal/webhook"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "status" {
		addr := "localhost:6060"
		if len(os.Args) > 2 {
			addr = os.Args[2]
		}
		p := tea.NewProgram(tui.NewModel(addr), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: ParseLogLevel(cfg.LogLevel),
	})))

	// Build app registry and per-app state
	registry := app.NewRegistry()
	managers := make(map[string]*channel.Manager)
	connections := make(map[string]*state.ConnectionManager)
	dispatchers := make(map[string]*webhook.Dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, ac := range cfg.Apps {
		a := &app.App{
			ID:                     ac.ID,
			Key:                    ac.Key,
			Secret:                 ac.Secret,
			WebhookURL:             ac.WebhookURL,
			ClientEvents:           ac.ClientEvents,
			MaxConnections:         ac.MaxConnections,
			MaxClientEventsPerSec:  ac.MaxClientEventsPerSec,
			MaxBackendEventsPerSec: ac.MaxBackendEventsPerSec,
		}
		registry.Add(a)
		managers[a.ID] = channel.NewManager()
		connections[a.ID] = state.NewConnectionManager(a.MaxConnections)

		d := webhook.NewDispatcher(a)
		dispatchers[a.ID] = d
		d.Start(ctx)

		slog.Info("app registered", "id", a.ID, "key", a.Key)
	}

	// Routes
	mux := http.NewServeMux()

	// WebSocket endpoint
	wsHandler := &server.WSHandler{
		Apps:           registry,
		Managers:       managers,
		Connections:    connections,
		MaxMessageSize: cfg.MaxMessageSize,
		AllowedOrigins: cfg.AllowedOrigins,
	}
	mux.Handle("GET /app/{appKey}", wsHandler)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// HTTP API
	apiRouter := api.NewRouter(registry, managers, cfg.AllowedOrigins)
	mux.Handle("/apps/", apiRouter)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Internal stats server (bound to localhost only)
	statsMux := http.NewServeMux()
	statsHandler := &api.StatsHandler{
		Apps:        registry,
		Managers:    managers,
		Connections: connections,
	}
	statsMux.Handle("GET /stats", statsHandler)
	statsAddr := fmt.Sprintf("127.0.0.1:%d", cfg.StatsPort)
	statsSrv := &http.Server{
		Addr:    statsAddr,
		Handler: statsMux,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		slog.Info("shutting down...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := statsSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("stats server shutdown error", "error", err)
		}
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	go func() {
		slog.Info("stats server started", "addr", statsAddr)
		if err := statsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("stats server error", "error", err)
		}
	}()

	if cfg.TLSCert != "" {
		slog.Info("gocket started", "addr", addr, "tls", true)
		err = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		slog.Info("gocket started", "addr", addr)
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
