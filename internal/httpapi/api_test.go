package httpapi

import (
	"encoding/json"
	"github.com/DAS27/driver-shift-diary/internal/diary"
	"github.com/DAS27/driver-shift-diary/web"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"
)

const valid = `{"id":"t1","start":"2026-10-01T08:10:00+05:00","end":"2026-10-01T08:32:00+05:00","amount":2400,"payment":"card","commission":360}`

func handler(t *testing.T) http.Handler {
	t.Helper()
	s, e := diary.Open(filepath.Join(t.TempDir(), "trips.json"))
	if e != nil {
		t.Fatal(e)
	}
	loc, e := time.LoadLocation("Asia/Almaty")
	if e != nil {
		t.Fatal(e)
	}
	return New(s, loc, web.Files)
}
func request(h http.Handler, method, path, body, content string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", content)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestAPIFlow(t *testing.T) {
	h := handler(t)
	for i, want := range []int{201, 200} {
		w := request(h, "POST", "/api/trips", valid, "application/json")
		if w.Code != want {
			t.Fatalf("attempt %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w := request(h, "GET", "/api/day?date=2026-10-01", "", "")
	var day struct {
		Trips   []diary.Trip  `json:"trips"`
		Summary diary.Summary `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &day); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(day.Trips) != 1 || day.Summary.Net != 204000 {
		t.Fatal(w.Body.String())
	}
	w = request(h, "POST", "/api/trips", strings.Replace(valid, "2400", "2401", 1), "application/json")
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/day?date=2026-10-03", "", "")
	if !strings.Contains(w.Body.String(), `"trips":[]`) {
		t.Fatal(w.Body.String())
	}
	w = request(h, "GET", "/api/days", "", "")
	if !strings.Contains(w.Body.String(), `"days":["2026-10-01"]`) {
		t.Fatal(w.Body.String())
	}
	w = request(h, "GET", "/", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Дневник водителя") {
		t.Fatal("client not served")
	}
}
func TestInvalidRequests(t *testing.T) {
	h := handler(t)
	for name, body := range map[string]string{"unknown": strings.Replace(valid, `"id"`, `"unknown"`, 1), "missing": strings.Replace(valid, `,"commission":360`, "", 1), "null": strings.Replace(valid, "360", "null", 1), "negative": strings.Replace(valid, "2400", "-1", 1), "trailing": valid + valid, "fraction": strings.Replace(valid, "2400", "1.001", 1), "end": strings.Replace(valid, "08:32", "08:10", 1), "payment": strings.Replace(valid, "card", "wire", 1), "array": "[]"} {
		t.Run(name, func(t *testing.T) {
			w := request(h, "POST", "/api/trips", body, "application/json")
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, date := range []string{"", "2026-02-30", "01-10-2026"} {
		if w := request(h, "GET", "/api/day?date="+date, "", ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if w := request(h, "POST", "/api/trips", valid, "text/plain"); w.Code != 415 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/api/trips", strings.Repeat("x", 17<<10), "application/json"); w.Code != 413 {
		t.Fatal(w.Code)
	}
}

func TestMethodsAndUnknownRoutes(t *testing.T) {
	h := handler(t)
	for _, test := range []struct {
		method, path string
		status       int
		allow        string
	}{
		{"GET", "/api/trips", 405, "POST"}, {"POST", "/api/day", 405, "GET, HEAD"},
		{"GET", "/api/missing", 404, ""}, {"DELETE", "/", 405, "GET, HEAD"},
	} {
		w := request(h, test.method, test.path, "", "")
		if w.Code != test.status || w.Header().Get("Allow") != test.allow {
			t.Errorf("%s %s: %d %s", test.method, test.path, w.Code, w.Body.String())
		}
	}
}
