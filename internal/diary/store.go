package diary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var ErrConflict = errors.New("id already belongs to a different trip")

// Store supports concurrent requests in one process. It commits to disk before publishing in memory.
type Store struct {
	mu        sync.RWMutex
	path      string
	trips     []Trip
	byID      map[string]int
	byContent map[string]int
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, trips: []Trip{}, byID: map[string]int{}, byContent: map[string]int{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		if err = save(path, s.trips); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil || raw == nil {
		return nil, fmt.Errorf("data file must contain a JSON array")
	}
	for i, r := range raw {
		t, e := DecodeTrip(r)
		if e != nil {
			return nil, fmt.Errorf("trip %d: %w", i, e)
		}
		if t.ID == "" {
			return nil, fmt.Errorf("trip %d: id is required in data file", i)
		}
		if _, ok := s.byID[t.ID]; ok {
			return nil, fmt.Errorf("duplicate id %q in data file", t.ID)
		}
		if _, ok := s.byContent[t.Fingerprint()]; ok {
			return nil, fmt.Errorf("duplicate trip in data file at index %d", i)
		}
		s.byID[t.ID] = i
		s.byContent[t.Fingerprint()] = i
		s.trips = append(s.trips, t)
	}
	return s, nil
}
func (s *Store) Add(t Trip) (Trip, bool, error) {
	if err := t.Validate(); err != nil {
		return Trip{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fingerprint := t.Fingerprint()
	if i, ok := s.byID[t.ID]; ok {
		if s.trips[i].Fingerprint() != fingerprint {
			return Trip{}, false, ErrConflict
		}
		return s.trips[i], false, nil
	}
	if i, ok := s.byContent[fingerprint]; ok {
		return s.trips[i], false, nil
	}
	if t.ID == "" {
		hash := sha256.Sum256([]byte(fingerprint))
		t.ID = "t_" + hex.EncodeToString(hash[:])
	}
	if _, ok := s.byID[t.ID]; ok {
		return Trip{}, false, ErrConflict
	}
	next := make([]Trip, len(s.trips)+1)
	copy(next, s.trips)
	next[len(s.trips)] = t
	if err := save(s.path, next); err != nil {
		return Trip{}, false, err
	}
	s.byID[t.ID] = len(s.trips)
	s.byContent[fingerprint] = len(s.trips)
	s.trips = next
	return t, true, nil
}
func (s *Store) Day(date string, loc *time.Location) []Trip {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []Trip{}
	for _, t := range s.trips {
		if t.Start.In(loc).Format(time.DateOnly) == date {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Start.Equal(result[j].Start) {
			return result[i].ID < result[j].ID
		}
		return result[i].Start.Before(result[j].Start)
	})
	return result
}
func (s *Store) Days(loc *time.Location) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	days := []string{}
	seen := map[string]bool{}
	for _, t := range s.trips {
		day := t.Start.In(loc).Format(time.DateOnly)
		if !seen[day] {
			seen[day] = true
			days = append(days, day)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}
func save(path string, trips []Trip) error {
	data, err := json.MarshalIndent(trips, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".trips-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Same-directory rename makes readers see either the previous or complete new file.
	return os.Rename(f.Name(), path)
}
