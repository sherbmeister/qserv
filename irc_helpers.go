package main

import (
	"fmt"
	"time"
)

// Send a NOTICE from our service *client* to a target (UID or #chan).
func (l *Link) NoticeFromService(target, text string) {
	_ = l.SendRaw(":%s NOTICE %s :%s", l.Cfg.ServiceUID, target, text)
}

// Send a PRIVMSG from our service *client* to a channel.
func (l *Link) ChanMsg(channel, text string) {
	_ = l.SendRaw(":%s PRIVMSG %s :%s", l.Cfg.ServiceUID, channel, text)
}

// Server-sourced MODE for users.
func (l *Link) ServerMode(target, modes string, args ...string) {
	line := fmt.Sprintf(":%s MODE %s %s", l.Cfg.SID, target, modes)
	for _, a := range args {
		line += " " + a
	}
	_ = l.SendRaw(line)
}

// Channel FMODE (TS-aware).
func (l *Link) FMode(ts int64, channel, modes string, args ...string) {
	line := fmt.Sprintf(":%s FMODE %s %d %s", l.Cfg.SID, channel, ts, modes)
	for _, a := range args {
		line += " " + a
	}
	_ = l.SendRaw(line)
}

// Try IJOIN (services-style), then fall back to FJOIN.
// - IJOIN:  :<sid> IJOIN <uid> <#chan> [ts] [modes]
// - FJOIN:  :<sid> FJOIN <#chan> <ts> + :[prefix,]UID:1
func (l *Link) ServiceJoinWithTS(channel string, knownTS int64, giveOp bool) {
	ts := knownTS
	if ts == 0 {
		ts = time.Now().Unix()
	}
	// Prefer IJOIN (if module is present it will just work)
	if giveOp {
		_ = l.SendRaw(":%s IJOIN %s %s %d +o", l.Cfg.SID, l.Cfg.ServiceUID, channel, ts)
	} else {
		_ = l.SendRaw(":%s IJOIN %s %s %d", l.Cfg.SID, l.Cfg.ServiceUID, channel, ts)
	}
	// Also send FJOIN as a fallback to ensure presence
	member := l.Cfg.ServiceUID + ":1"
	if giveOp {
		member = "o," + member
	}
	_ = l.SendRaw(":%s FJOIN %s %d + :%s", l.Cfg.SID, channel, ts, member)
}

// Add a WHOIS line using server metadata.
// This makes /whois show something like “is a Network Service”.
func (l *Link) SetServiceWhois(uid, line string) {
	_ = l.SendRaw(":%s METADATA %s swhois :%s", l.Cfg.SID, uid, line)
}
