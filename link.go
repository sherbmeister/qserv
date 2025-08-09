package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type Link struct {
	Cfg    Config
	Logger *Logger
	Bus    *Bus

	mu   sync.Mutex
	conn net.Conn
	w    *bufio.Writer
}

func (l *Link) ConnectAndRun(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", l.Cfg.Host, l.Cfg.Port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var (
		d   net.Conn
		err error
	)
	if l.Cfg.TLS {
		d, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		d, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer d.Close()

	l.mu.Lock()
	l.conn = d
	l.w = bufio.NewWriter(d)
	l.mu.Unlock()

	l.Logger.Infof("connected to %s", addr)

	// ✨ IMPORTANT: reset handlers on every (re)connect BEFORE any handshake registers them
	l.Bus = NewBus(l.Logger)

	// proto handshake
	switch strings.ToLower(l.Cfg.Protocol) {
	case "insp4":
		if err := Insp4Handshake(l); err != nil {
			return err
		}
	case "p10":
		if err := P10Handshake(l); err != nil {
			return err
		}
	}

	// reader
	errCh := make(chan error, 1)
	go func() { errCh <- l.readLoop(ctx) }()

	// introduce service (Insp does it from handler after ENDBURST)
	switch strings.ToLower(l.Cfg.Protocol) {
	case "p10":
		if err := P10IntroduceService(l); err != nil {
			l.Logger.Errorf("introduce service failed: %v", err)
		}
	}

	// keepalive
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case <-tick.C:
			// periodic ping to keep NATs happy; the server will also ping us (handled in insp_core.go)
			_ = l.SendRaw("PING :qserv")
		}
	}
}

func (l *Link) readLoop(ctx context.Context) error {
	sc := bufio.NewScanner(l.conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		l.Logger.Debugf("< %s", line)
		msg := ParseLine(line)
		if msg != nil && l.Bus != nil {
			l.Bus.Emit(l, msg)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("server closed the connection")
}

func (l *Link) SendRaw(format string, a ...any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return fmt.Errorf("not connected")
	}
	line := fmt.Sprintf(format, a...)
	if !strings.HasSuffix(line, "\r\n") {
		line += "\r\n"
	}
	l.Logger.Debugf("> %s", strings.TrimRight(line, "\r\n"))
	if _, err := l.w.WriteString(line); err != nil {
		return err
	}
	return l.w.Flush()
}
