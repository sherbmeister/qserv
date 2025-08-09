// suspend.go
package main

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

type Suspension struct {
	Until  int64  `json:"until"` // unix seconds
	Reason string `json:"reason"`
}

type SuspendStore struct {
	mu    sync.RWMutex
	path  string
	Chans map[string]Suspension `json:"chans"` // lower(#chan) -> suspension
	Accs  map[string]Suspension `json:"accs"`  // lower(account) -> suspension
}

func NewSuspendStore(path string) *SuspendStore {
	return &SuspendStore{
		path:  path,
		Chans: map[string]Suspension{},
		Accs:  map[string]Suspension{},
	}
}

func (s *SuspendStore) Load() error {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(s)
}

func (s *SuspendStore) Save() error {
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *SuspendStore) IsAccSuspended(acc string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a := s.Accs[strings.ToLower(acc)]
	return a.Until > time.Now().Unix()
}

func (s *SuspendStore) IsChanSuspended(ch string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.Chans[strings.ToLower(ch)]
	return c.Until > time.Now().Unix()
}

func (s *SuspendStore) SuspendAcc(acc string, until int64, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Accs[strings.ToLower(acc)] = Suspension{Until: until, Reason: reason}
}

func (s *SuspendStore) UnsuspendAcc(acc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Accs, strings.ToLower(acc))
}

func (s *SuspendStore) SuspendChan(ch string, until int64, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Chans[strings.ToLower(ch)] = Suspension{Until: until, Reason: reason}
}

func (s *SuspendStore) PurgeChan(ch string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Chans, strings.ToLower(ch))
}
