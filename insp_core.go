// insp_core.go
package main

func registerInspCoreHandlers(l *Link) {
	l.Bus.On("PING", func(link *Link, msg *Message) {
		switch len(msg.Params) {
		case 2:
			src := msg.Params[0] // their SID
			dst := msg.Params[1] // our SID
			link.Logger.Debugf("[ping] PING %s %s -> PONG %s %s", src, dst, dst, src)
			_ = link.SendRaw("PONG %s %s", src, dst)
		case 1:
			// :src PING dst
			if msg.Prefix != "" && msg.Params[0] != "" {
				src := msg.Prefix
				if src[0] == ':' {
					src = src[1:]
				}
				dst := msg.Params[0]
				link.Logger.Debugf("[ping] :%s PING %s -> PONG %s %s", src, dst, dst, src)
				_ = link.SendRaw("PONG %s %s", dst, src)
				return
			}
			// Token-style fallback (PING :token)
			fallthrough
		default:
			token := ""
			if len(msg.Params) > 0 {
				token = msg.Params[len(msg.Params)-1]
			}
			if token == "" {
				token = "qserv"
			}
			link.Logger.Debugf("[ping] token PING :%s -> PONG :%s", token, token)
			_ = link.SendRaw("PONG :%s", token)
		}
	})

	l.Bus.On("ERROR", func(link *Link, msg *Message) {
		if link.Logger != nil {
			link.Logger.Errorf("uplink ERROR: %v", msg.Params)
		}
	})

	l.Bus.On("ENDBURST", func(link *Link, _ *Message) {
		if link.Logger != nil {
			link.Logger.Debugf("received ENDBURST")
		}
	})
}
