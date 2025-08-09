package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type chanState struct {
	TS   int64
	Seen map[string]bool // uid -> present
}

var (
	qStore  = NewStore("state.json")
	acl     = NewChanACLStore("chan_access.json")
	suspend = NewSuspendStore("suspended.json")

	chans        = make(map[string]*chanState)
	serviceChans = []string{"#feds", "#services", "#opers"}
	opers        = make(map[string]bool) // uid -> oper
)

func registerServiceHandlers(l *Link) {
	_ = qStore.Load()
	_ = accDB.Load()
	_ = acl.Load()
	_ = suspend.Load()

	// Track FJOIN / JOIN / PART / QUIT / OPERTYPE (same as before) ...
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
	l.Bus.On("UID", func(_ *Link, m *Message) {
		if len(m.Params) >= 4 {
			setNick(m.Params[0], m.Params[3])
		}
	})
	l.Bus.On("NICK", func(_ *Link, m *Message) {
		if len(m.Params) >= 1 && m.Prefix != "" {
			setNick(m.Prefix, m.Params[0])
		}
	})
	l.Bus.On("JOIN", func(_ *Link, m *Message) {
		if len(m.Params) < 1 || m.Prefix == "" {
			return
		}
		key := toLower(m.Params[0])
		cs := chans[key]
		if cs == nil {
			cs = &chanState{Seen: map[string]bool{}}
			chans[key] = cs
		}
		cs.Seen[m.Prefix] = true
	})
	l.Bus.On("PART", func(_ *Link, m *Message) {
		if len(m.Params) >= 1 && m.Prefix != "" {
			if cs := chans[toLower(m.Params[0])]; cs != nil {
				delete(cs.Seen, m.Prefix)
			}
		}
	})
	l.Bus.On("QUIT", func(_ *Link, m *Message) {
		uid := m.Prefix
		if uid == "" {
			return
		}
		for _, cs := range chans {
			delete(cs.Seen, uid)
		}
		delNick(uid)
		delete(opers, uid)
	})
	l.Bus.On("OPERTYPE", func(_ *Link, m *Message) {
		if m.Prefix != "" {
			opers[m.Prefix] = true
		}
	})

	l.Bus.On("ENDBURST", func(l *Link, _ *Message) {
		l.SetServiceWhois(l.Cfg.ServiceUID, "is a Network Service")
		l.ServerMode(l.Cfg.ServiceUID, "+Bk")
		for _, ch := range serviceChans {
			var ts int64
			if cs := chans[toLower(ch)]; cs != nil {
				ts = cs.TS
			}
			l.ServiceJoinWithTS(ch, ts, true)
			time.Sleep(50 * time.Millisecond)
		}
	})

	// Debug tap
	l.Bus.On("PRIVMSG", func(_ *Link, m *Message) {
		l.Logger.Debugf("[tap] PRIVMSG from %s to %v | %q", m.Prefix, m.Params, m.Trailing)
	})

	// Dispatcher (channel and PM)
	l.Bus.On("PRIVMSG", func(l *Link, m *Message) {
		if len(m.Params) == 0 {
			return
		}
		target := m.Params[0]
		text := strings.TrimSpace(m.Trailing)
		from := m.Prefix
		if text == "" || from == "" {
			return
		}

		if strings.HasPrefix(target, "#") {
			// channel: require '!'
			if !strings.HasPrefix(text, "!") {
				return
			}
			handleChannelControl(l, from, target, strings.TrimSpace(text[1:]))
			return
		}
		// PM: execute PM verbs, and also accept channel verbs WITHOUT '!'
		handlePM(l, from, text)
	})
}

// ----- Helpers -----

func enforceNotSuspendedPM(l *Link, uid string) bool {
	acc := accDB.SessionAccount(uid)
	if acc != "" && suspend.IsAccSuspended(acc) {
		l.NoticeFromService(uid, "Your account is suspended and cannot use Q commands.")
		return false
	}
	return true
}

func canControlChannel(l *Link, uid, channel string, min int) (ok bool, level int, isOwner bool) {
	if suspend.IsChanSuspended(channel) {
		return false, 0, false
	}
	acc := accDB.SessionAccount(uid)
	if acc == "" {
		return false, 0, false
	}
	level = acl.GetLevel(channel, acc)
	owner, hasOwner := acl.Owner(channel)
	isOwner = hasOwner && owner == acc
	ok = isOwner || level >= min
	return
}

// ----- Channel commands (to channel) -----

func handleChannelControl(l *Link, fromUID, channel, cmdline string) {
	if !enforceNotSuspendedPM(l, fromUID) {
		return
	}

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
		if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
			l.ChanMsg(channel, "You do not have access on "+channel+".")
			return
		}
		l.FMode(cs.TS, channel, "+o", fromUID)

	case "deop":
		if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
			l.ChanMsg(channel, "You do not have access on "+channel+".")
			return
		}
		l.FMode(cs.TS, channel, "-o", fromUID)

	case "voice":
		if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
			l.ChanMsg(channel, "You do not have access on "+channel+".")
			return
		}
		l.FMode(cs.TS, channel, "+v", fromUID)

	case "devoice":
		if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
			l.ChanMsg(channel, "You do not have access on "+channel+".")
			return
		}
		l.FMode(cs.TS, channel, "-v", fromUID)

	case "adduser":
		// adduser <account|nick> <level>
		ok, _, isOwner := canControlChannel(l, fromUID, channel, 400)
		if !ok && !isOwner {
			l.ChanMsg(channel, "Only the owner can change access.")
			return
		}
		if len(parts) < 3 {
			l.ChanMsg(channel, "Usage: !adduser <account|nick> <level (1-500)>")
			return
		}
		target := parts[1]
		lvl, _ := strconv.Atoi(parts[2])
		if lvl < 1 || lvl > 500 {
			l.ChanMsg(channel, "Level must be between 1 and 500.")
			return
		}
		tacct := resolveAccountFromToken(target)
		if tacct == "" {
			l.ChanMsg(channel, "Unable to resolve target to an account.")
			return
		}
		acl.SetLevel(channel, tacct, lvl)
		_ = acl.Save()
		l.ChanMsg(channel, "Set access: "+tacct+" = "+strconv.Itoa(lvl))

	case "deluser":
		ok, _, isOwner := canControlChannel(l, fromUID, channel, 400)
		if !ok && !isOwner {
			l.ChanMsg(channel, "Only the owner can delete access.")
			return
		}
		if len(parts) < 2 {
			l.ChanMsg(channel, "Usage: !deluser <account|nick>")
			return
		}
		tacct := resolveAccountFromToken(parts[1])
		if tacct == "" {
			l.ChanMsg(channel, "Unable to resolve target to an account.")
			return
		}
		acl.DelUser(channel, tacct)
		_ = acl.Save()
		l.ChanMsg(channel, "Removed access for "+tacct)

	case "access", "flags", "listaccess":
		entries := acl.List(channel)
		if len(entries) == 0 {
			l.ChanMsg(channel, "No access entries.")
			return
		}
		var b strings.Builder
		b.WriteString("Access: ")
		for i, e := range entries {
			if i > 0 {
				b.WriteString(" | ")
			}
			fmt.Fprintf(&b, "%s=%d", e.Account, e.Level)
		}
		l.ChanMsg(channel, b.String())

	case "join":
		if ok, _, _ := canControlChannel(l, fromUID, channel, 450); !ok {
			l.ChanMsg(channel, "Need level 450+ to JOIN.")
			return
		}
		l.ServiceJoinWithTS(channel, cs.TS, true)

	case "part":
		if ok, _, _ := canControlChannel(l, fromUID, channel, 450); !ok {
			l.ChanMsg(channel, "Need level 450+ to PART.")
			return
		}
		_ = l.SendRaw(":%s PART %s :Requested", l.Cfg.ServiceUID, channel)

	case "purge":
		// IRCop only
		if !opers[fromUID] {
			l.ChanMsg(channel, "IRCop only.")
			return
		}
		doPurge(l, channel)
		l.ChanMsg(channel, "Purged "+channel)

	case "suspend":
		// IRCop only: suspend #chan <days> <reason...>
		if !opers[fromUID] {
			l.ChanMsg(channel, "IRCop only.")
			return
		}
		if len(parts) < 3 {
			l.ChanMsg(channel, "Usage: !suspend #chan <days> <reason>")
			return
		}
		days, _ := strconv.Atoi(parts[1])
		reason := strings.TrimSpace(strings.Join(parts[2:], " "))
		doSuspendChannel(l, channel, days, reason)
		l.ChanMsg(channel, "Suspended "+channel+" for "+strconv.Itoa(days)+" day(s): "+reason)

	default:
		// ignore
	}
}

// ----- PM commands -----

func handlePM(l *Link, fromUID, cmdline string) {
	if !enforceNotSuspendedPM(l, fromUID) {
		return
	}

	parts := strings.Fields(cmdline)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "ping":
		l.NoticeFromService(fromUID, "pong")

	// account register/login/logout (unchanged)
	case "register":
		if len(parts) < 3 {
			l.NoticeFromService(fromUID, "Usage: register <account> <password>")
			return
		}
		acc, pass := parts[1], parts[2]
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
		acc, pass := parts[1], parts[2]
		if !accDB.Verify(acc, pass) {
			l.NoticeFromService(fromUID, "Login failed.")
			return
		}
		accDB.Bind(fromUID, acc)
		l.SetAccount(fromUID, acc)
		l.SetVHost(fromUID, acc+".users.emechnet.org")
		l.NoticeFromService(fromUID, "You are now logged in as "+acc+".")

	case "logout":
		accDB.Unbind(fromUID)
		l.ClearAccount(fromUID)
		l.NoticeFromService(fromUID, "You are now logged out.")

	// channel registration (normal user or IRCop form)
	case "regchannel":
		if len(parts) < 2 || !strings.HasPrefix(parts[1], "#") {
			l.NoticeFromService(fromUID, "Usage: regchannel <#channel> [owneraccount]")
			return
		}
		channel := parts[1]
		var owner string
		if len(parts) >= 3 && opers[fromUID] {
			owner = parts[2]
		} else {
			if !accDB.IsAuthed(fromUID) {
				l.NoticeFromService(fromUID, "Login required.")
				return
			}
			owner = accDB.SessionAccount(fromUID)
		}
		if owner == "" {
			l.NoticeFromService(fromUID, "Could not determine owner account.")
			return
		}
		if _, exists := qStore.GetChan(channel); exists {
			l.NoticeFromService(fromUID, channel+" is already registered.")
			return
		}
		if _, created := qStore.PutChan(channel, owner); created {
			acl.SetOwner(channel, owner)
			_ = acl.Save()
			_ = qStore.Save()
			l.NoticeFromService(fromUID, "Registered "+channel+" — owner: "+owner)
			return
		}
		l.NoticeFromService(fromUID, "Could not register "+channel+".")

	// PM *versions* of channel controls
	case "op", "deop", "voice", "devoice", "adduser", "deluser", "access", "join", "part", "purge", "suspend":
		// require a channel as second arg
		if len(parts) < 2 || !strings.HasPrefix(parts[1], "#") {
			l.NoticeFromService(fromUID, "Usage: "+cmd+" <#channel> [...args]")
			return
		}
		channel := parts[1]
		switch cmd {
		case "access":
			entries := acl.List(channel)
			if len(entries) == 0 {
				l.NoticeFromService(fromUID, "No access entries for "+channel+".")
				return
			}
			var b strings.Builder
			for i, e := range entries {
				if i > 0 {
					b.WriteString(" | ")
				}
				fmt.Fprintf(&b, "%s=%d", e.Account, e.Level)
			}
			l.NoticeFromService(fromUID, "Access for "+channel+": "+b.String())

		case "adduser":
			if len(parts) < 4 {
				l.NoticeFromService(fromUID, "Usage: adduser <#channel> <account|nick> <level (1-500)>")
				return
			}
			if ok, _, isOwner := canControlChannel(l, fromUID, channel, 400); !ok && !isOwner {
				l.NoticeFromService(fromUID, "Only the owner can change access.")
				return
			}
			tacct := resolveAccountFromToken(parts[2])
			lvl, _ := strconv.Atoi(parts[3])
			if tacct == "" || lvl < 1 || lvl > 500 {
				l.NoticeFromService(fromUID, "Bad target or level.")
				return
			}
			acl.SetLevel(channel, tacct, lvl)
			_ = acl.Save()
			l.NoticeFromService(fromUID, "Set access on "+channel+": "+tacct+" = "+strconv.Itoa(lvl))

		case "deluser":
			if len(parts) < 3 {
				l.NoticeFromService(fromUID, "Usage: deluser <#channel> <account|nick>")
				return
			}
			if ok, _, isOwner := canControlChannel(l, fromUID, channel, 400); !ok && !isOwner {
				l.NoticeFromService(fromUID, "Only the owner can delete access.")
				return
			}
			tacct := resolveAccountFromToken(parts[2])
			if tacct == "" {
				l.NoticeFromService(fromUID, "Bad target.")
				return
			}
			acl.DelUser(channel, tacct)
			_ = acl.Save()
			l.NoticeFromService(fromUID, "Removed access on "+channel+" for "+tacct)

		case "op":
			if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
				l.NoticeFromService(fromUID, "No access on "+channel+".")
				return
			}
			if cs := chans[toLower(channel)]; cs != nil {
				l.FMode(cs.TS, channel, "+o", fromUID)
			}

		case "deop":
			if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
				l.NoticeFromService(fromUID, "No access on "+channel+".")
				return
			}
			if cs := chans[toLower(channel)]; cs != nil {
				l.FMode(cs.TS, channel, "-o", fromUID)
			}

		case "voice":
			if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
				l.NoticeFromService(fromUID, "No access on "+channel+".")
				return
			}
			if cs := chans[toLower(channel)]; cs != nil {
				l.FMode(cs.TS, channel, "+v", fromUID)
			}

		case "devoice":
			if ok, _, _ := canControlChannel(l, fromUID, channel, 1); !ok {
				l.NoticeFromService(fromUID, "No access on "+channel+".")
				return
			}
			if cs := chans[toLower(channel)]; cs != nil {
				l.FMode(cs.TS, channel, "-v", fromUID)
			}

		case "join":
			if ok, _, _ := canControlChannel(l, fromUID, channel, 450); !ok {
				l.NoticeFromService(fromUID, "Need level 450+ to JOIN.")
				return
			}
			if cs := chans[toLower(channel)]; cs != nil {
				l.ServiceJoinWithTS(channel, cs.TS, true)
			}

		case "part":
			if ok, _, _ := canControlChannel(l, fromUID, channel, 450); !ok {
				l.NoticeFromService(fromUID, "Need level 450+ to PART.")
				return
			}
			_ = l.SendRaw(":%s PART %s :Requested", l.Cfg.ServiceUID, channel)

		case "purge":
			if !opers[fromUID] {
				l.NoticeFromService(fromUID, "IRCop only.")
				return
			}
			doPurge(l, channel)
			l.NoticeFromService(fromUID, "Purged "+channel)

		case "suspend":
			if !opers[fromUID] || len(parts) < 4 {
				l.NoticeFromService(fromUID, "Usage: suspend <#channel> <days> <reason>")
				return
			}
			days, _ := strconv.Atoi(parts[2])
			reason := strings.TrimSpace(strings.Join(parts[3:], " "))
			doSuspendChannel(l, channel, days, reason)
			l.NoticeFromService(fromUID, "Suspended "+channel+" for "+strconv.Itoa(days)+" day(s): "+reason)
		}

	default:
		l.NoticeFromService(fromUID, "Unknown command: "+cmd)
	}
}

// Resolve token into an account (nick->UID->account), else assume it's already an account.
func resolveAccountFromToken(tok string) string {
	if uid := uidFromNick(tok); uid != "" {
		if a := accDB.SessionAccount(uid); a != "" {
			return a
		}
	}
	return tok
}

// Admin ops

func doPurge(l *Link, channel string) {
	// leave channel
	_ = l.SendRaw(":%s PART %s :Purged", l.Cfg.ServiceUID, channel)

	// delete ACL + suspension
	delete(acl.data, strings.ToLower(channel))
	_ = acl.Save()
	suspend.PurgeChan(channel)
	_ = suspend.Save()

	// forget cached TS/Seen
	delete(chans, toLower(channel))

	// best-effort save of remaining state
	_ = qStore.Save()
}

func doSuspendChannel(l *Link, channel string, days int, reason string) {
	if days < 1 {
		days = 1
	}
	until := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	// mark channel
	suspend.SuspendChan(channel, until, reason)
	// suspend all accounts in its ACL (incl owner)
	for _, e := range acl.List(channel) {
		suspend.SuspendAcc(e.Account, until, "Suspended via "+channel+": "+reason)
	}
	_ = suspend.Save()
}
