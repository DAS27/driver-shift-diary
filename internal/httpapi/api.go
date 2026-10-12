// Package httpapi exposes the diary over HTTP and serves the embedded client.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/DAS27/driver-shift-diary/internal/diary"
)

func New(store *diary.Store, loc *time.Location, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/days", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"days": store.Days(loc), "timezone": loc.String(), "today": time.Now().In(loc).Format(time.DateOnly)})
	})
	mux.HandleFunc("GET /api/day", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			fail(w, 400, "invalid_date", "date must use YYYY-MM-DD format")
			return
		}
		trips := store.Day(date, loc)
		respond(w, 200, map[string]any{"date": date, "timezone": loc.String(), "trips": trips, "summary": diary.Summarize(trips)})
	})
	mux.HandleFunc("POST /api/trips", func(w http.ResponseWriter, r *http.Request) {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			fail(w, 415, "unsupported_media_type", "Content-Type must be application/json")
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if err != nil {
			fail(w, 413, "body_too_large", "request body exceeds 16 KiB")
			return
		}
		trip, err := diary.DecodeTrip(data)
		if err != nil {
			fail(w, 400, "invalid_trip", err.Error())
			return
		}
		trip, created, err := store.Add(trip)
		if errors.Is(err, diary.ErrConflict) {
			fail(w, 409, "id_conflict", err.Error())
			return
		}
		if err != nil {
			slog.Error("save trip", "error", err)
			fail(w, 500, "storage_error", "could not save trip; retry the same request")
			return
		}
		status := 200
		if created {
			status = 201
		}
		respond(w, status, map[string]any{"trip": trip, "created": created})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/day", "/api/days":
			w.Header().Set("Allow", "GET, HEAD")
			fail(w, 405, "method_not_allowed", "method not allowed")
		case "/api/trips":
			w.Header().Set("Allow", "POST")
			fail(w, 405, "method_not_allowed", "method not allowed")
		default:
			fail(w, 404, "not_found", "API endpoint not found")
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	files := http.FileServer(http.FS(assets))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			fail(w, 405, "method_not_allowed", "method not allowed")
			return
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
