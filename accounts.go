package main

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Account struct {
	Name      string `json:"name"`
	Hash      []byte `json:"hash"`
	CreatedTS int64  `json:"created_ts"`
}

type accountState struct {
	Accounts map[string]Account `json:"accounts"` // key: lowercased name
}

type AccountDB struct {
	file string
	mu   sync.RWMutex
	s    accountState
	// live sessions: UID -> account name (original case)
	sessions map[string]string
}

var accDB = NewAccountDB("accounts.json")

func NewAccountDB(path string) *AccountDB {
	return &AccountDB{
		file:     path,
		s:        accountState{Accounts: make(map[string]Account)},
		sessions: make(map[string]string),
	}
}

func (db *AccountDB) Load() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	b, err := os.ReadFile(db.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var st accountState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.Accounts == nil {
		st.Accounts = make(map[string]Account)
	}
	db.s = st
	return nil
}

func (db *AccountDB) Save() error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	b, err := json.MarshalIndent(db.s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(db.file, b, 0600)
}

func (db *AccountDB) Create(name, password string) error {
	if name == "" || password == "" {
		return errors.New("empty account or password")
	}
	lname := toLower(name)
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, exists := db.s.Accounts[lname]; exists {
		return errors.New("account already exists")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	db.s.Accounts[lname] = Account{
		Name:      name,
		Hash:      hash,
		CreatedTS: time.Now().Unix(),
	}
	return nil
}

func (db *AccountDB) Verify(name, password string) bool {
	lname := toLower(name)
	db.mu.RLock()
	acct, ok := db.s.Accounts[lname]
	db.mu.RUnlock()
	if !ok {
		return false
	}
	return bcrypt.CompareHashAndPassword(acct.Hash, []byte(password)) == nil
}

func (db *AccountDB) Bind(uid, account string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.sessions[uid] = account
}

func (db *AccountDB) Unbind(uid string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.sessions, uid)
}

func (db *AccountDB) IsAuthed(uid string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	_, ok := db.sessions[uid]
	return ok
}

func (db *AccountDB) SessionAccount(uid string) string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.sessions[uid]
}
