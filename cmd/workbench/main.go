package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alkval/workbench/internal/config"
	processmanager "github.com/alkval/workbench/internal/process"
	"github.com/alkval/workbench/internal/server"
	"github.com/alkval/workbench/internal/store"
	"github.com/alkval/workbench/internal/webui"
)

func main() {
	output := io.Writer(os.Stdout)
	if directory, err := executableDirectory(); err == nil {
		if file, openErr := os.OpenFile(filepath.Join(directory, "workbench.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); openErr == nil {
			defer file.Close()
			output = io.MultiWriter(os.Stdout, file)
		}
	}
	logger := slog.New(slog.NewJSONHandler(output, nil))
	if err := run(logger); err != nil {
		logger.Error("workbench stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	directory, _ := executableDirectory()
	configFallback := filepath.Join("config", "services.windows.json")
	dataFallback := "data"
	if directory != "" {
		if _, err := os.Stat(filepath.Join(directory, "services.json")); err == nil {
			configFallback = filepath.Join(directory, "services.json")
			dataFallback = filepath.Join(directory, "data")
		}
	}
	configPath := env("WORKBENCH_CONFIG", configFallback)
	dataDir := env("WORKBENCH_DATA_DIR", dataFallback)
	address := env("WORKBENCH_ADDRESS", "0.0.0.0:8787")
	password := os.Getenv("WORKBENCH_PASSWORD")
	if password == "" && directory != "" {
		contents, err := os.ReadFile(filepath.Join(directory, "password.txt"))
		if err == nil {
			contents = bytes.TrimPrefix(contents, []byte{0xEF, 0xBB, 0xBF})
			password = string(contents)
		}
	}
	if len(password) < 10 {
		return errors.New("WORKBENCH_PASSWORD must contain at least 10 characters")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	eventStore, err := store.Open(filepath.Join(dataDir, "workbench.db"))
	if err != nil {
		return err
	}
	defer eventStore.Close()
	manager := processmanager.New(cfg, eventStore)
	secureCookie := os.Getenv("WORKBENCH_INSECURE_COOKIE") != "true"
	handler := server.New(manager, eventStore, webui.Files(), password, secureCookie, logger).Handler()
	httpServer := &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	logger.Info("workbench listening", "address", address, "services", len(cfg.Services))
	err = httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func executableDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}
