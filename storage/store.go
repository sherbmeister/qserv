package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	PasswordHash string   `json:"password_hash"`
	Registered   string   `json:"registered"`
	LastSeen     string   `json:"last_seen"`
	Channels     []string `json:"channels"`
}

var userFile = "storage/users.json"
var users = map[string]User{}

func init() {
	loadUsers()
}

func loadUsers() {
	file, err := os.ReadFile(userFile)
	if err != nil {
		fmt.Println("No user database found, starting fresh.")
		return
	}
	json.Unmarshal(file, &users)
}

func saveUsers() {
	data, _ := json.MarshalIndent(users, "", "  ")
	_ = os.WriteFile(userFile, data, 0644)
}

func RegisterUser(account string, password string) error {
	if _, exists := users[account]; exists {
		return fmt.Errorf("account already exists")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	users[account] = User{
		PasswordHash: string(hash),
		Registered:   time.Now().UTC().Format(time.RFC3339),
		LastSeen:     time.Now().UTC().Format(time.RFC3339),
	}
	saveUsers()
	return nil
}

func AuthenticateUser(account string, password string) bool {
	user, exists := users[account]
	if !exists {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil
}

func UpdateLastSeen(account string) {
	user, ok := users[account]
	if !ok {
		return
	}
	user.LastSeen = time.Now().UTC().Format(time.RFC3339)
	users[account] = user
	saveUsers()
}

func GetUserInfo(account string) string {
	user, ok := users[account]
	if !ok {
		return ""
	}
	return fmt.Sprintf("Registered: %s | Last Seen: %s | Channels: %v", user.Registered, user.LastSeen, user.Channels)
}
