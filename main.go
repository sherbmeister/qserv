// qserv/main.go
package main

import (
	"fmt"
	"log"
	"strings"

	irc "github.com/thoj/go-ircevent"
	"qserv/storage"
)

const (
	ircServer = "127.0.0.1:6667"       // UnrealIRCd server
	ircNick = "Q"
	ircUser = "Q"
	ircRealName = "QServ"
	ircChannel = "#help"
)

func main() {
	conn := irc.IRC(ircNick, ircUser)
	conn.RealName = ircRealName
	conn.UseTLS = false

	conn.AddCallback("001", func(e *irc.Event) {
		conn.Join(ircChannel)
	})

	conn.AddCallback("PRIVMSG", func(e *irc.Event) {
		user := e.Nick + "@" + strings.SplitN(e.Source, "@", 2)[1]
		args := strings.Fields(e.Message())
		if len(args) == 0 {
			return
		}

		switch strings.ToUpper(args[0]) {
		case "REGISTER":
			if len(args) < 2 {
				conn.Privmsg(e.Nick, "Usage: REGISTER <password>")
				return
			}
			pass := args[1]
			err := storage.RegisterUser(user, pass)
			if err != nil {
				conn.Privmsg(e.Nick, fmt.Sprintf("Registration failed: %v", err))
			} else {
				conn.Privmsg(e.Nick, "Account registered successfully.")
			}
		case "LOGIN":
			if len(args) < 2 {
				conn.Privmsg(e.Nick, "Usage: LOGIN <password>")
				return
			}
			pass := args[1]
			ok := storage.AuthenticateUser(user, pass)
			if ok {
				conn.Privmsg(e.Nick, "Login successful.")
				storage.UpdateLastSeen(user)
			} else {
				conn.Privmsg(e.Nick, "Login failed. Invalid password or account.")
			}
		case "INFO":
			info := storage.GetUserInfo(user)
			if info == "" {
				conn.Privmsg(e.Nick, "No account found.")
			} else {
				conn.Privmsg(e.Nick, info)
			}
		}
	})

	err := conn.Connect(ircServer)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	conn.Loop()
}
