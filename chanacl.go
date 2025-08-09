package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
)

type ChanACL struct {
	Owner  string         `json:"owner"`  // account name
	Levels map[string]int `json:"levels"` // account -> level (1..500)
}

type ChanACLStore struct {
	path string
	mu   sync.RWMutex
	data map[string]*ChanACL // lowercased #chan -> acl
}

func NewChanACLStore(path string) *ChanACLStore {
	return &ChanACLStore{
		path: path,
		data: make(map[string]*ChanACL),
	}
}

func (s *ChanACLStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	return dec.Decode(&s.data)
}

func (s *ChanACLStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.data); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Ensure entry exists
func (s *ChanACLStore) ensure(ch string) *ChanACL {
	key := strings.ToLower(ch)
	acl := s.data[key]
	if acl == nil {
		acl = &ChanACL{Levels: make(map[string]int)}
		s.data[key] = acl
	}
	return acl
}

func (s *ChanACLStore) SetOwner(channel, account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account = strings.ToLower(account) // ← normalize
	acl := s.ensure(channel)
	acl.Owner = account
	if acl.Levels == nil {
		acl.Levels = make(map[string]int)
	}
	acl.Levels[account] = 500
}

func (s *ChanACLStore) Owner(channel string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acl := s.data[strings.ToLower(channel)]
	if acl == nil || acl.Owner == "" {
		return "", false
	}
	return acl.Owner, true
}

func (s *ChanACLStore) SetLevel(channel, account string, level int) {
	if level < 0 {
		level = 0
	}
	if level > 500 {
		level = 500
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	account = strings.ToLower(account) // ← normalize
	acl := s.ensure(channel)
	if level == 0 {
		if account != acl.Owner {
			delete(acl.Levels, account)
		} else {
			acl.Levels[account] = 500
		}
		return
	}
	acl.Levels[account] = level
}

func (s *ChanACLStore) GetLevel(channel, account string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	account = strings.ToLower(account) // ← normalize
	acl := s.data[strings.ToLower(channel)]
	if acl == nil {
		return 0
	}
	if account == acl.Owner {
		return 500
	}
	return acl.Levels[account]
}

func (s *ChanACLStore) DelUser(channel, account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account = strings.ToLower(account) // ← normalize
	acl := s.data[strings.ToLower(channel)]
	if acl == nil {
		return
	}
	if account == acl.Owner {
		return
	} // don’t remove owner here
	delete(acl.Levels, account)
}

type AccessEntry struct {
	Account string `json:"account"`
	Level   int    `json:"level"`
}

func (s *ChanACLStore) Channels() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for ch := range s.data {
		if strings.HasPrefix(ch, "#") { // safety
			out = append(out, ch)
		}
	}
	return out
}

func (s *ChanACLStore) List(channel string) []AccessEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acl := s.data[strings.ToLower(channel)]
	if acl == nil {
		return nil
	}
	out := make([]AccessEntry, 0, len(acl.Levels))
	for acc, lvl := range acl.Levels {
		out = append(out, AccessEntry{Account: acc, Level: lvl})
	}
	// make sure owner is present & level 500
	if acl.Owner != "" {
		found := false
		for i := range out {
			if out[i].Account == acl.Owner {
				out[i].Level = 500
				found = true
				break
			}
		}
		if !found {
			out = append(out, AccessEntry{Account: acl.Owner, Level: 500})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level == out[j].Level {
			return out[i].Account < out[j].Account
		}
		return out[i].Level > out[j].Level
	})
	return out
}
