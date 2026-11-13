// vercel-control owns a Node instance's stream leases over loopback. The parent
// keeps stdin open; parent exit closes it and shuts down the control process.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/server"
)

func main() {
	if err := run(); err != nil {
		config.Logger.Error("control process failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	stateDir, err := os.MkdirTemp("", "ds2api-control-")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(stateDir); err != nil {
			config.Logger.Warn("remove control state failed", "error", err)
		}
	}()
	for key, name := range map[string]string{
		"DS2API_CHAT_HISTORY_PATH": "history.json", "DS2API_USAGE_PATH": "usage.json",
		"DS2API_TOOL_DIAGNOSTICS_PATH": "diagnostics.json", "DS2API_ACCOUNT_DAILY_LIMIT_PATH": "daily.json",
	} {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, filepath.Join(stateDir, name)); err != nil {
				return err
			}
		}
	}
	app, err := server.NewApp()
	if err != nil {
		return err
	}
	defer app.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: app.Router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second}
	defer func() {
		if err := srv.Close(); err != nil {
			config.Logger.Warn("close control server failed", "error", err)
		}
	}()
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopped)
	parentClosed := make(chan struct{})
	go func() {
		_, err := io.Copy(io.Discard, os.Stdin)
		if err != nil {
			config.Logger.Warn("control parent pipe failed", "error", err)
		}
		close(parentClosed)
	}()
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"origin": "http://" + listener.Addr().String()}); err != nil {
		return err
	}
	select {
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			return err
		}
	case <-stopped:
	case <-parentClosed:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
