package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ----- wire helpers you already used -----

func (l *Link) SetServiceWhois(uid, text string) {
	if uid == "" || text == "" {
		return
	}
	_ = l.SendRaw(":%s METADATA %s swhois :%s", l.Cfg.SID, uid, text)
}

func (l *Link) ServerMode(uid, modes string, args ...string) {
	if uid == "" || modes == "" {
		return
	}
	line := fmt.Sprintf(":%s MODE %s %s", l.Cfg.SID, uid, modes)
	for _, a := range args {
		line += " " + a
	}
	_ = l.SendRaw(line)
}

// Join as Q, normalizing TS to seconds (never 0), then self-op via FMODE (server prefix).
func (l *Link) ServiceJoinWithTS(channel string, ts int64, giveOp bool) {
	// Normalize TS to seconds and ensure non-zero
	tsSec := ts
	if tsSec <= 0 {
		tsSec = time.Now().Unix()
	}
	if tsSec > 1_000_000_000_000 { // looks like milliseconds
		tsSec = tsSec / 1000
	}

	// Send IJOIN as the service user (UID prefix)
	_ = l.SendRaw(":%s IJOIN %s %d", l.Cfg.ServiceUID, channel, tsSec)

	// Cache/remember the TS so future FMODEs don’t use 0
	key := toLower(channel)
	cs := chans[key]
	if cs == nil {
		cs = &chanState{TS: tsSec, Seen: map[string]bool{}}
		chans[key] = cs
	} else if cs.TS == 0 {
		cs.TS = tsSec
	}
	if cs.Seen == nil {
		cs.Seen = map[string]bool{}
	}
	cs.Seen[l.Cfg.ServiceUID] = true

	if giveOp {
		// FMODE MUST use the channel TS (seconds) and come from the SERVER SID
		_ = l.SendRaw(":%s FMODE %s %d +o %s", l.Cfg.SID, channel, cs.TS, l.Cfg.ServiceUID)
	}
}

func (l *Link) FMode(chanTS int64, channel, modes string, args ...string) {
	line := fmt.Sprintf(":%s FMODE %s %d %s", l.Cfg.SID, channel, chanTS, modes)
	for _, a := range args {
		line += " " + a
	}
	_ = l.SendRaw(line)
}

func (l *Link) NoticeFromService(targetUID, text string) {
	if targetUID == "" || text == "" {
		return
	}
	_ = l.SendRaw(":%s NOTICE %s :%s", l.Cfg.ServiceUID, targetUID, text)
}

func (l *Link) ChanMsg(channel, text string) {
	if channel == "" || text == "" {
		return
	}
	_ = l.SendRaw(":%s NOTICE %s :%s", l.Cfg.ServiceUID, channel, text)
}

// ----- new: account + vhost helpers -----

// SetAccount attaches an account to a user and sets +r.
func (l *Link) SetAccount(uid, account string) {
	if uid == "" || account == "" {
		return
	}
	_ = l.SendRaw(":%s METADATA %s accountname :%s", l.Cfg.SID, uid, account)
	_ = l.SendRaw(":%s MODE %s +r", l.Cfg.SID, uid)
}

// ClearAccount removes +r and clears the accountname metadata.
func (l *Link) ClearAccount(uid string) {
	if uid == "" {
		return
	}
	_ = l.SendRaw(":%s MODE %s -r", l.Cfg.SID, uid)
	// Clearing accountname: send empty or omit; we’ll set a blank to be explicit
	_ = l.SendRaw(":%s METADATA %s accountname :", l.Cfg.SID, uid)
}

// SetVHost sets user's displayed host (requires m_chghost).
func (l *Link) SetVHost(uid, vhost string) {
	if uid == "" || vhost == "" {
		return
	}
	_ = l.SendRaw(":%s CHGHOST %s %s", l.Cfg.SID, uid, vhost)
}

// ----- optional: simple nick <-> uid map (JOIN/NICK/QUIT tracking) -----

var (
	nickMu   sync.RWMutex
	uid2nick = map[string]string{} // uid -> nick
	nick2uid = map[string]string{} // lower(nick) -> uid
)

func uidFromNick(nick string) string {
	nickMu.RLock()
	defer nickMu.RUnlock()
	return nick2uid[strings.ToLower(nick)]
}

// User-triggered mode changes: have Q (service user) set the modes, not the server.
func (l *Link) QChanMode(channel, modes string, targetUID string) {
	if targetUID != "" {
		_ = l.SendRaw(":%s MODE %s %s %s", l.Cfg.ServiceUID, channel, modes, targetUID)
	} else {
		_ = l.SendRaw(":%s MODE %s %s", l.Cfg.ServiceUID, channel, modes)
	}
}
func requireLoginForChannel(l *Link, fromUID, channel string) (string, bool) {
	acc := accDB.SessionAccount(fromUID)
	if acc == "" {
		l.ChanMsg(channel, "Login required. Use: /msg Q login <account> <password>")
		return "", false
	}
	if suspend.IsAccSuspended(acc) {
		l.ChanMsg(channel, "Your account is suspended and cannot use Q commands.")
		return "", false
	}
	return acc, true
}
func getOr(slice []string, idx int, def string) string {
	if idx >= 0 && idx < len(slice) {
		return slice[idx]
	}
	return def
}
func (l *Link) SetAccountMetaOnly(uid, account string) {
	if uid == "" || account == "" {
		return
	}
	_ = l.SendRaw(":%s METADATA %s accountname :%s", l.Cfg.SID, uid, account)
}
