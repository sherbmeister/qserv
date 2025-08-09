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

func (l *Link) ServiceJoinWithTS(channel string, chanTS int64, opSelf bool) {
	uid := l.Cfg.ServiceUID
	if uid == "" {
		return
	}
	membid := time.Now().UnixNano() / 1e6
	_ = l.SendRaw(":%s IJOIN %s %d", uid, channel, membid)
	if opSelf {
		_ = l.SendRaw(":%s FMODE %s %d +o %s", l.Cfg.SID, channel, chanTS, uid)
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
	_ = l.SendRaw(":%s PRIVMSG %s :%s", l.Cfg.ServiceUID, channel, text)
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

func setNick(uid, nick string) {
	nickMu.Lock()
	defer nickMu.Unlock()
	old := uid2nick[uid]
	if old != "" {
		delete(nick2uid, strings.ToLower(old))
	}
	uid2nick[uid] = nick
	nick2uid[strings.ToLower(nick)] = uid
}

func delNick(uid string) {
	nickMu.Lock()
	defer nickMu.Unlock()
	if old, ok := uid2nick[uid]; ok {
		delete(nick2uid, strings.ToLower(old))
	}
	delete(uid2nick, uid)
}

func uidFromNick(nick string) string {
	nickMu.RLock()
	defer nickMu.RUnlock()
	return nick2uid[strings.ToLower(nick)]
}
