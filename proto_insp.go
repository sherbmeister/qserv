package main

import "time"

// Insp4Handshake performs the InspIRCd v4 server handshake and introduces our
// service client (Q) inside our own burst.
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

	// 4) Introduce Q (service client) inside the burst.

	// Ensure ServiceUID is 9 chars: SID (3) + 6.
	if len(l.Cfg.ServiceUID) != 9 || l.Cfg.ServiceUID[:3] != l.Cfg.SID {
		l.Cfg.ServiceUID = l.Cfg.SID + "AAAAAA"
	}
	uid := l.Cfg.ServiceUID
	ts := now
	signon := now

	// v4 UID order (IMPORTANT):
	// :<sid> UID <uid> <ts> <nick> <real-host> <displayed-host> <real-user> <displayed-user> <ip> <signon> <modes> :<real>
	if err := l.SendRaw(":%s UID %s %d %s %s %s %s %s %s %d +Bk :%s",
		l.Cfg.SID, // prefix
		uid,       // <uid>
		ts,        // <ts>
		l.Cfg.QNick,
		l.Cfg.QHost, // real-host
		l.Cfg.QHost, // displayed-host
		l.Cfg.QUser, // real-user
		l.Cfg.QUser, // displayed-user
		"0.0.0.0",   // ip
		signon,      // signon
		l.Cfg.QReal, // :real/gecos
	); err != nil {
		return err
	}

	// 5) End *our* burst
	if err := l.SendRaw(":%s ENDBURST", l.Cfg.SID); err != nil {
		return err
	}

	// 6) Register handlers
	registerInspCoreHandlers(l)
	registerServiceHandlers(l)
	return nil
}
