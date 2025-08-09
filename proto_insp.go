package main

import (
	"time"
)

// Insp4Handshake performs the InspIRCd v4 server handshake and introduces our
// service client (Q) *inside our own burst* so subsequent messages that refer
// to Q (MODE/METADATA/IJOIN) are valid.
func Insp4Handshake(l *Link) error {
	now := time.Now().Unix()

	// 1) CAPAB
	if err := l.SendRaw("CAPAB START 1206"); err != nil {
		return err
	}
	if err := l.SendRaw("CAPAB END"); err != nil {
		return err
	}

	// 2) SERVER <name> <pass> <sid> :<desc>
	if err := l.SendRaw("SERVER %s %s %s :%s",
		l.Cfg.ServerName, l.Cfg.Password, l.Cfg.SID, l.Cfg.ServerDesc); err != nil {
		return err
	}

	// 3) Start *our* burst
	if err := l.SendRaw(":%s BURST %d", l.Cfg.SID, now); err != nil {
		return err
	}

	// 4) Introduce Q (service client) *inside* the burst.
	// UID format (InspIRCd v4):
	// :<sid> UID <uid> <ts> <nick> <host> <vhost> <ident> <realhost> <ip> <signon_ts> <modes> :<gecos>
	if l.Cfg.ServiceUID == "" {
		l.Cfg.ServiceUID = l.Cfg.SID + "AAAAAAA"
	}
	uidts := now
	signon := now

	if err := l.SendRaw(":%s UID %s %d %s %s %s %s %s %s %d +Bk :%s",
		l.Cfg.SID,
		l.Cfg.ServiceUID,
		uidts,
		l.Cfg.QNick, // nick: e.g. "Q"
		l.Cfg.QHost, // host
		l.Cfg.QHost, // vhost
		l.Cfg.QUser, // ident
		l.Cfg.QHost, // realhost (reuse host)
		"0.0.0.0",   // ip placeholder
		signon,
		l.Cfg.QReal, // gecos/realname
	); err != nil {
		return err
	}

	// 5) End *our* burst
	if err := l.SendRaw(":%s ENDBURST", l.Cfg.SID); err != nil {
		return err
	}

	// 6) Register handlers (core + service), being careful not to double register.
	registerInspCoreHandlers(l)
	registerServiceHandlers(l)

	return nil
}
