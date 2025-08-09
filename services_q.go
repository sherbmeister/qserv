package main

import (
	"strconv"
	"strings"
	"time"
)

// Live channel state learned from FJOIN
type chanState struct {
	TS   int64
	Seen map[string]bool
}

var (
	qStore       = NewStore("state.json")
	chans        = make(map[string]*chanState)              // key: lowercased #chan
	serviceChans = []string{"#feds", "#services", "#opers"} // tweak as needed
)

// Q's service handlers (called from Insp4Handshake)
func registerServiceHandlers(l *Link) {
	// load state once (idempotent)
	_ = qStore.Load()
	_ = accDB.Load() // accounts.json

	// Track FJOIN to learn channel TS and present members
	l.Bus.On("FJOIN", func(l *Link, m *Message) {
		if len(m.Params) < 3 {
			return
		}
		ch := m.Params[0]
		ts, _ := strconv.ParseInt(m.Params[1], 10, 64)
		key := toLower(ch)
		cs := chans[key]
		if cs == nil {
			cs = &chanState{TS: ts, Seen: map[string]bool{}}
			chans[key] = cs
		} else if cs.TS == 0 {
			cs.TS = ts
		}
		for _, entry := range strings.Fields(m.Trailing) {
			uid := entry
			if i := strings.IndexByte(uid, ','); i >= 0 {
				uid = uid[i+1:]
			}
			if i := strings.IndexByte(uid, ':'); i >= 0 {
				uid = uid[:i]
			}
			if uid != "" {
				cs.Seen[uid] = true
			}
		}
	})

	// After uplink ENDBURST: mark as service, set modes, join channels using TS
	l.Bus.On("ENDBURST", func(l *Link, _ *Message) {
		// WHOIS marker and modes (+B bot, +k servprotect)
		l.SetServiceWhois(l.Cfg.ServiceUID, "is a Network Service")
		l.ServerMode(l.Cfg.ServiceUID, "+Bk")

		// Join service channels, using known TS when we have it
		for _, ch := range serviceChans {
			key := toLower(ch)
			var ts int64
			if cs := chans[key]; cs != nil {
				ts = cs.TS
			}
			l.ServiceJoinWithTS(ch, ts, true) // +o Q on join
			time.Sleep(50 * time.Millisecond)
		}
	})

	// Debug tap for incoming PRIVMSG
	l.Bus.On("PRIVMSG", func(_ *Link, m *Message) {
		l.Logger.Debugf("[tap] PRIVMSG from %s to %v | %q", m.Prefix, m.Params, m.Trailing)
	})

	// Command dispatcher
	l.Bus.On("PRIVMSG", func(l *Link, m *Message) {
		if len(m.Params) == 0 {
			return
		}
		target := m.Params[0]
		text := strings.TrimSpace(m.Trailing)
		from := m.Prefix // UID

		if text == "" || from == "" {
			return
		}

		if strings.HasPrefix(target, "#") {
			// CHANNEL: only !commands, and only channel-control ones.
			if !strings.HasPrefix(text, "!") {
				return
			}
			handleChannelControl(l, from, target, strings.TrimSpace(text[1:]))
			return
		}

		// PM: NO leading "!"
		if strings.HasPrefix(text, "!") {
			l.NoticeFromService(from, "Use commands without '!' in PM. Example: register myname mypass")
			return
		}
		handlePM(l, from, text)
	})
}

// Channel-only control commands; replies go TO THE CHANNEL
func handleChannelControl(l *Link, fromUID, channel, cmdline string) {
	parts := strings.Fields(cmdline)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])

	key := toLower(channel)
	cs := chans[key]
	if cs == nil || cs.TS == 0 {
		l.ChanMsg(channel, "I don't know the TS for "+channel+" yet; try again shortly.")
		return
	}

	switch cmd {
	case "op":
		l.FMode(cs.TS, channel, "+o", fromUID)
		l.ChanMsg(channel, "Opped.")
	case "deop":
		l.FMode(cs.TS, channel, "-o", fromUID)
		l.ChanMsg(channel, "De-opped.")
	case "voice":
		l.FMode(cs.TS, channel, "+v", fromUID)
		l.ChanMsg(channel, "Voiced.")
	case "devoice":
		l.FMode(cs.TS, channel, "-v", fromUID)
		l.ChanMsg(channel, "De-voiced.")
	default:
		// ignore unknowns in channel
	}
}

// PM-only commands; replies by NOTICE
// Accounts:
//
//	register <account> <password>
//	login <account> <password>
//	logout
//
// Channel registration (requires login):
//
//	register #channel
//	op #channel   (convenience)
func handlePM(l *Link, fromUID, cmdline string) {
	parts := strings.Fields(cmdline)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "ping":
		l.NoticeFromService(fromUID, "pong")

	case "register":
		// account or channel register depending on arg
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "#") {
			// channel register (requires login)
			channel := parts[1]
			if !accDB.IsAuthed(fromUID) {
				l.NoticeFromService(fromUID, "You must be logged in to register channels. Use: login <account> <password>")
				return
			}
			if _, exists := qStore.GetChan(channel); exists {
				l.NoticeFromService(fromUID, channel+" is already registered.")
				return
			}
			if _, created := qStore.PutChan(channel, accDB.SessionAccount(fromUID)); created {
				_ = qStore.Save()
				l.NoticeFromService(fromUID, "Registered "+channel+" — owner set to your account.")
			}
			return
		}
		// account register
		if len(parts) < 3 {
			l.NoticeFromService(fromUID, "Usage: register <account> <password>")
			return
		}
		acc := parts[1]
		pass := parts[2]
		if err := accDB.Create(acc, pass); err != nil {
			l.NoticeFromService(fromUID, "Register failed: "+err.Error())
			return
		}
		_ = accDB.Save()
		l.NoticeFromService(fromUID, "Account registered. You can now: login "+acc+" <password>")

	case "login":
		if len(parts) < 3 {
			l.NoticeFromService(fromUID, "Usage: login <account> <password>")
			return
		}
		acc := parts[1]
		pass := parts[2]
		if !accDB.Verify(acc, pass) {
			l.NoticeFromService(fromUID, "Login failed.")
			return
		}
		accDB.Bind(fromUID, acc)
		l.NoticeFromService(fromUID, "You are now logged in as "+acc+".")

	case "logout":
		accDB.Unbind(fromUID)
		l.NoticeFromService(fromUID, "You are now logged out.")

	case "op":
		// convenience: op #channel
		if len(parts) < 2 || !strings.HasPrefix(parts[1], "#") {
			l.NoticeFromService(fromUID, "Usage: op #channel")
			return
		}
		channel := parts[1]
		key := toLower(channel)
		cs := chans[key]
		if cs == nil || cs.TS == 0 {
			l.NoticeFromService(fromUID, "I don't know the TS for "+channel+" yet; try again shortly.")
			return
		}
		l.FMode(cs.TS, channel, "+o", fromUID)
		l.NoticeFromService(fromUID, "You are opped in "+channel+".")

	default:
		l.NoticeFromService(fromUID, "Unknown command: "+cmd)
	}
}
