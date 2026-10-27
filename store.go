package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Campaign is one fundraiser. Donations go to its own subaddress, so totals
// never mix between campaigns even though they share one wallet.
type Campaign struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Goal        uint64    `json:"goal"` // atomic units
	Address     string    `json:"address"`
	SubIndex    uint32    `json:"sub_index"`
	CreatedAt   time.Time `json:"created_at"`
}

// Store keeps campaigns in a single JSON file. Enough for an MVP; no cgo, no DB.
type Store struct {
	path string
	mu   sync.RWMutex
	byID map[string]Campaign
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, byID: map[string]Campaign{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Campaign
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, c := range list {
		s.byID[c.ID] = c
	}
	return s, nil
}

func (s *Store) Get(id string) (Campaign, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.byID[id]
	return c, ok
}

// List returns campaigns newest first.
func (s *Store) List() []Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Campaign, 0, len(s.byID))
	for _, c := range s.byID {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list
}

func (s *Store) Add(c Campaign) error {
	if !slugRe.MatchString(c.ID) {
		return fmt.Errorf("id %q: use 2-63 chars of a-z, 0-9 and '-'", c.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.byID[c.ID]; dup {
		return fmt.Errorf("campaign %q already exists", c.ID)
	}
	s.byID[c.ID] = c
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	list := make([]Campaign, 0, len(s.byID))
	for _, c := range s.byID {
		list = append(list, c)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
