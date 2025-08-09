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

	remoteSID string
	lastRx    int64 // unix nano of last successful read
}

func (l *Link) ConnectAndRun(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", l.Cfg.Host, l.Cfg.Port)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

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
	defer func() { _ = d.Close() }()

	if tc, ok := d.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
		_ = tc.SetNoDelay(true)
	}
	_ = d.SetDeadline(time.Time{})
	_ = d.SetReadDeadline(time.Time{})
	_ = d.SetWriteDeadline(time.Time{})

	l.mu.Lock()
	l.conn = d
	l.w = bufio.NewWriterSize(d, 64*1024)
	l.mu.Unlock()

	l.Logger.Infof("connected to %s", addr)
	l.lastRx = time.Now().UnixNano()

	// New bus for this connection
	l.Bus = NewBus(l.Logger)

	// Handshake
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

	// Reader
	errCh := make(chan error, 1)
	go func() { errCh <- l.readLoop(ctx) }()

	// P10 introduces immediately; Insp4 introduces Q inside ENDBURST handler
	if strings.ToLower(l.Cfg.Protocol) == "p10" {
		if err := P10IntroduceService(l); err != nil {
			l.Logger.Errorf("introduce service failed: %v", err)
		}
	}

	// No proactive PINGs — server drives cadence.

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		}
	}
}

func (l *Link) readLoop(ctx context.Context) error {
	l.mu.Lock()
	c := l.conn
	l.mu.Unlock()
	if c == nil {
		return fmt.Errorf("readLoop: not connected")
	}

	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Text()
		l.lastRx = time.Now().UnixNano()
		l.Logger.Debugf("< %s", line)

		// ---- RAW PING/PONG FAST-PATH (pre-parse) ----
		// Handle all PINGs here to avoid any param-order bugs in handlers.

		if strings.HasPrefix(line, "PING ") {
			// Token style: "PING :token"
			tok := strings.TrimSpace(line[len("PING "):])
			if tok != "" {
				_ = l.SendRaw("PONG %s", tok) // keep ":" if present
				l.lastRx = time.Now().UnixNano()
				continue
			}
		}

		if strings.HasPrefix(line, ":") && strings.Contains(line, " PING ") {
			// Server style: ":<srcSID> PING <dstSID>[:token]"
			parts := strings.SplitN(line, " ", 4)
			if len(parts) >= 3 && strings.EqualFold(parts[1], "PING") {
				srcSID := strings.TrimPrefix(parts[0], ":")
				dstSID := parts[2]
				// Ignore echoes that appear to originate from us.
				if strings.EqualFold(srcSID, l.Cfg.SID) {
					l.Logger.Debugf("[ping] ignoring echo from our SID (%s)", srcSID)
					continue
				}
				// Only reply if it's targeting us.
				if strings.EqualFold(dstSID, l.Cfg.SID) {
					_ = l.SendRaw("PONG %s %s", srcSID, l.Cfg.SID)
					l.lastRx = time.Now().UnixNano()
					continue
				}
			}
		}

		if strings.HasPrefix(line, "PONG ") || strings.Contains(line, " PONG ") {
			// Any inbound PONG means the link is alive; just refresh lastRx.
			l.lastRx = time.Now().UnixNano()
			// Do not emit to Bus; nothing to handle.
		}

		// ---- end raw fast-path ----

		// Parsed path
		msg := ParseLine(line)
		if msg == nil {
			continue
		}

		// Capture remote SID from SERVER once
		if strings.EqualFold(msg.Command, "SERVER") && len(msg.Params) >= 3 {
			l.remoteSID = msg.Params[2] // e.g., "034"
		}

		// Dispatch async
		go l.Bus.Emit(l, msg)
	}

	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("server closed the connection")
}

// SendRaw writes one IRC line with a short write deadline and flush.
func (l *Link) SendRaw(format string, a ...any) error {
	line := fmt.Sprintf(format, a...)
	if !strings.HasSuffix(line, "\r\n") {
		line += "\r\n"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil || l.w == nil {
		return fmt.Errorf("not connected")
	}

	_ = l.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	l.Logger.Debugf("> %s", strings.TrimRight(line, "\r\n"))

	if _, err := l.w.WriteString(line); err != nil {
		return err
	}
	if err := l.w.Flush(); err != nil {
		return err
	}
	_ = l.conn.SetWriteDeadline(time.Time{})
	return nil
}
