package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type ChannelReg struct {
	Name      string `json:"name"`
	OwnerUID  string `json:"owner_uid"`
	CreatedTS int64  `json:"created_ts"`
}

type State struct {
	Channels map[string]ChannelReg `json:"channels"`
}

type Store struct {
	file  string
	state State
	mu    sync.RWMutex
}

func NewStore(file string) *Store {
	return &Store{
		file: file,
		state: State{
			Channels: make(map[string]ChannelReg),
		},
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.Channels == nil {
		st.Channels = make(map[string]ChannelReg)
	}
	s.state = st
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, b, 0644)
}

func (s *Store) GetChan(name string) (ChannelReg, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cr, ok := s.state.Channels[toLower(name)]
	return cr, ok
}

func (s *Store) PutChan(name, ownerUID string) (ChannelReg, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := toLower(name)
	if _, exists := s.state.Channels[k]; exists {
		return s.state.Channels[k], false
	}
	cr := ChannelReg{
		Name:      name,
		OwnerUID:  ownerUID,
		CreatedTS: time.Now().Unix(),
	}
	s.state.Channels[k] = cr
	return cr, true
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if 'A' <= b[i] && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
