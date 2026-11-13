package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/multi-proxmox-manager/internal/config"
	"example.com/multi-proxmox-manager/internal/database"
	"example.com/multi-proxmox-manager/internal/httpapi"
	"example.com/multi-proxmox-manager/internal/proxmox"
	"example.com/multi-proxmox-manager/internal/security"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.MySQLDSN)
	if err != nil {
		logger.Error("database connection failed; verify DSN, connectivity and permissions")
		os.Exit(1)
	}
	defer db.Close()

	box, err := security.NewSecretBox(cfg.EncryptionKey)
	if err != nil {
		logger.Error("credential encryption initialization failed", "error", err)
		os.Exit(1)
	}
	pve, err := proxmox.NewClient(cfg.AllowedCIDRs, cfg.PVECACertFile)
	if err != nil {
		logger.Error("Proxmox client initialization failed", "error", err)
		os.Exit(1)
	}

	app := httpapi.New(db, cfg, box, pve)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      25 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("API server listening", "addr", cfg.Addr, "environment", cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			serveErrors <- err
		}
	}()

	failed := false
	select {
	case <-ctx.Done():
	case <-serveErrors:
		failed = true
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	if failed {
		os.Exit(1)
	}
}
