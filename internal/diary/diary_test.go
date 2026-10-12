package diary

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	_ "time/tzdata"
)

func sample() Trip {
	return Trip{ID: "t1", Start: time.Date(2026, 10, 1, 8, 10, 0, 0, time.FixedZone("UTC+5", 5*3600)), End: time.Date(2026, 10, 1, 8, 32, 0, 0, time.FixedZone("UTC+5", 5*3600)), Amount: 240000, Payment: "card", Commission: 36000}
}
func TestSummary(t *testing.T) {
	a := sample()
	b := a
	b.ID = "t2"
	b.Amount = 150000
	b.Commission = 22500
	b.Payment = "cash"
	s := Summarize([]Trip{a, b})
	if s.Totals != (Totals{2, 390000, 58500, 331500}) || s.Cash != (Totals{1, 150000, 22500, 127500}) || s.Card != (Totals{1, 240000, 36000, 204000}) {
		t.Fatalf("unexpected summary: %+v", s)
	}
	if Summarize(nil) != (Summary{}) {
		t.Fatal("empty day must be zero")
	}
	a.Amount = 101
	a.Commission = 10
	b.Amount = 202
	b.Commission = 20
	if s := Summarize([]Trip{a, b}); s.Revenue != 303 || s.Net != 273 {
		t.Fatalf("fractional money: %+v", s)
	}
}
func TestMoney(t *testing.T) {
	for input, want := range map[string]Money{"0": 0, "1.2": 120, "0.01": 1, "2400": 240000, "1000000000": MaxMoney} {
		var m Money
		if err := json.Unmarshal([]byte(input), &m); err != nil || m != want {
			t.Fatalf("%s: %d %v", input, m, err)
		}
		b, _ := json.Marshal(m)
		var again Money
		if err := json.Unmarshal(b, &again); err != nil || again != m {
			t.Fatal("round trip", string(b), err)
		}
	}
	for _, input := range []string{"null", "1.001", "1e3", "\"10\"", "1000000000.01", "9223372036854775807", "true"} {
		var m Money
		if err := json.Unmarshal([]byte(input), &m); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}
func TestValidation(t *testing.T) {
	for name, change := range map[string]func(*Trip){"zero amount": func(t *Trip) { t.Amount = 0 }, "negative": func(t *Trip) { t.Amount = -1 }, "equal time": func(t *Trip) { t.End = t.Start }, "earlier": func(t *Trip) { t.End = t.Start.Add(-time.Second) }, "payment": func(t *Trip) { t.Payment = "bank" }, "negative commission": func(t *Trip) { t.Commission = -1 }, "high commission": func(t *Trip) { t.Commission = t.Amount + 1 }, "id": func(t *Trip) { t.ID = "../file" }, "missing time": func(t *Trip) { t.Start = time.Time{} }} {
		t.Run(name, func(t *testing.T) {
			trip := sample()
			change(&trip)
			if trip.Validate() == nil {
				t.Fatal("accepted invalid trip")
			}
		})
	}
}
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trips.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}
func TestDuplicatesAndRestart(t *testing.T) {
	s, path := newStore(t)
	original, created, err := s.Add(sample())
	if err != nil || !created {
		t.Fatal(created, err)
	}
	other := sample()
	other.ID = "different"
	other.Start = other.Start.UTC()
	other.End = other.End.UTC()
	for _, trip := range []Trip{sample(), other} {
		got, created, err := s.Add(trip)
		if err != nil || created || got.ID != original.ID {
			t.Fatal(got, created, err)
		}
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	other.ID = ""
	if _, created, err = s.Add(other); err != nil || created {
		t.Fatal("duplicate after restart", created, err)
	}
	conflicting := sample()
	conflicting.Amount++
	if _, _, err = s.Add(conflicting); !errors.Is(err, ErrConflict) {
		t.Fatal("expected ID conflict", err)
	}
	if len(s.Day("2026-10-01", time.UTC)) != 1 {
		t.Fatal("unexpected count")
	}
}
func TestConcurrentRetries(t *testing.T) {
	s, _ := newStore(t)
	var wg sync.WaitGroup
	var created atomic.Int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			trip := sample()
			trip.ID = ""
			_, added, err := s.Add(trip)
			if err != nil {
				t.Error(err)
			}
			if added {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 {
		t.Fatalf("created %d times", created.Load())
	}
}
func TestDayBoundary(t *testing.T) {
	s, _ := newStore(t)
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		t.Fatal(err)
	}
	trip := sample()
	trip.Start = time.Date(2026, 10, 1, 19, 10, 0, 0, time.UTC)
	trip.End = trip.Start.Add(30 * time.Minute)
	if _, _, err = s.Add(trip); err != nil {
		t.Fatal(err)
	}
	if len(s.Day("2026-10-01", loc)) != 0 || len(s.Day("2026-10-02", loc)) != 1 {
		t.Fatal("incorrect local day")
	}
	trip.ID = "overnight"
	trip.Start = time.Date(2026, 10, 2, 23, 50, 0, 0, loc)
	trip.End = trip.Start.Add(30 * time.Minute)
	if _, _, err = s.Add(trip); err != nil {
		t.Fatal(err)
	}
	if len(s.Day("2026-10-02", loc)) != 2 || len(s.Day("2026-10-03", loc)) != 0 {
		t.Fatal("overnight must count on start day")
	}
}
func TestFailedWriteDoesNotPublish(t *testing.T) {
	s, path := newStore(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Add(sample()); err == nil {
		t.Fatal("expected storage failure")
	}
	if len(s.Day("2026-10-01", time.UTC)) != 0 {
		t.Fatal("failed write changed memory")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.Add(sample()); err != nil || !created {
		t.Fatal("retry should succeed", created, err)
	}
}
func TestCorruptDataRejected(t *testing.T) {
	for _, data := range []string{"null", "{}", "[{}]", "not json"} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trips.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(path); err == nil {
				t.Fatal("accepted corrupt file")
			}
		})
	}
}
