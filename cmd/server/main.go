package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/DAS27/driver-shift-diary/internal/diary"
	"github.com/DAS27/driver-shift-diary/internal/httpapi"
	"github.com/DAS27/driver-shift-diary/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	data := flag.String("data", "data/trips.json", "writable JSON data file")
	zone := flag.String("timezone", "Asia/Almaty", "IANA timezone for daily reports")
	flag.Parse()
	loc, err := time.LoadLocation(*zone)
	if err != nil {
		slog.Error("invalid timezone", "error", err)
		os.Exit(1)
	}
	store, err := diary.Open(*data)
	if err != nil {
		slog.Error("open diary", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: *addr, Handler: httpapi.New(store, loc, web.Files), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		slog.Info("diary ready", "url", "http://"+*addr, "timezone", *zone)
		done <- server.ListenAndServe()
	}()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			slog.Error("shutdown", "error", err)
			os.Exit(1)
		}
	}
}
