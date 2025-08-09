package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const p10b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789[]"

func b64(n, width int) string {
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = p10b64[n&63]
		n >>= 6
	}
	return string(buf)
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}

var (
	// Link target (snircd bind)
	host   = flag.String("host", env("Q_HOST", "127.0.0.1"), "snircd host")
	port   = flag.Int("port", envInt("Q_PORT", 4400), "snircd port")
	usetls = flag.Bool("tls", env("Q_TLS", "false") == "true", "use TLS (if stunnel/SSL listener)")

	// Our server identity
	name  = flag.String("name", env("Q_NAME", "services.emechnet.org"), "service server name")
	pass  = flag.String("pass", env("Q_PASS", "passwd"), "link password (matches C: line)")
	ss    = flag.String("ss", env("Q_SS", "AA"), "2-char P10 server numeric assigned in N: line")
	maxcc = flag.Int("max", envInt("Q_MAX", 262143), "max clients (builds SSCCC)")
	desc  = flag.String("desc", env("Q_DESC", "EmechNET IRC Services"), "server description")

	// Our pseudo-client (Q)
	introQ   = flag.Bool("intro", env("Q_INTRO", "true") == "true", "introduce Q client")
	nickQ    = flag.String("nick", env("Q_NICK", "Q"), "service nick")
	userQ    = flag.String("user", env("Q_USER", "qserv"), "ident/user")
	hostQ    = flag.String("vhost", env("Q_HOSTNAME", "services.emechnet.org"), "display host for Q")
	realQ    = flag.String("real", env("Q_REAL", "Channel Service"), "gecos/realname")
	fixedCCC = flag.String("ccc", os.Getenv("Q_CCC"), "optional fixed 3-char client numeric")

	// Accounts storage
	acctPath  = flag.String("accounts", env("Q_ACCOUNTS", "./accounts.json"), "path to accounts JSON store")
	vhPattern = flag.String("vhpat", env("Q_VHPAT", "{user}.emechnet.org"), "vhost pattern; use {user} placeholder")
)

// runtime state
var qNumeric string
var autochans = strings.Split(env("Q_AUTOJOIN", "#feds"), ",")

// --- simple account store ---
type Account struct {
	Name string `json:"name"`
	Salt string `json:"salt"`
	Hash string `json:"hash"` // hex(sha256(salt || password))
}

type store struct {
	Accounts map[string]Account `json:"accounts"` // by lower(account)
	Sessions map[string]string  `json:"sessions"` // numeric -> account (lower)
	Path     string             `json:"-"`
}

func (s *store) load() error {
	p := s.Path
	if p == "" {
		return fmt.Errorf("store path empty")
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		s.Accounts = map[string]Account{}
		s.Sessions = map[string]string{}
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if err := dec.Decode(s); err != nil {
		return err
	}
	if s.Accounts == nil {
		s.Accounts = map[string]Account{}
	}
	if s.Sessions == nil {
		s.Sessions = map[string]string{}
	}
	return nil
}

func (s *store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	tmp := s.Path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, s.Path)
}

func newSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashPass(salt, pass string) string {
	h := sha256.Sum256([]byte(salt + pass))
	return hex.EncodeToString(h[:])
}

var db store

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	flag.Parse()

	db.Path = *acctPath
	if err := db.load(); err != nil {
		log.Fatalf("failed to load accounts: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	dial := func(network, address string) (net.Conn, error) { return net.Dial(network, address) }
	if *usetls {
		dial = func(network, address string) (net.Conn, error) {
			return tls.Dial(network, address, &tls.Config{InsecureSkipVerify: true})
		}
	}

	retry := 0
	for {
		if err := linkOnce(dial, addr); err != nil {
			back := time.Duration(min(30, 1<<min(6, retry))) * time.Second
			log.Printf("link error: %v — reconnecting in %s", err, back)
			time.Sleep(back)
			retry++
			continue
		}
		return
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func linkOnce(dial func(string, string) (net.Conn, error), addr string) error {
	log.Printf("connecting to %s", addr)
	c, err := dial("tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()

	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	start := time.Now().Unix()

	// PASS + SERVER (P10 handshake)
	send(bw, fmt.Sprintf("PASS :%s", *pass))
	ssccc := *ss + b64(*maxcc, 3)
	send(bw, fmt.Sprintf("SERVER %s 1 %d %d J10 %s :%s", *name, start, start, ssccc, *desc))

	// Minimal burst: optionally introduce Q then EB
	if *introQ {
		introduceQ(bw, start)
	}
	send(bw, fmt.Sprintf("%s EB", *ss)) // end of burst

	linked := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		log.Printf("< %s", line)

		switch verb(line) {
		case "G": // PING -> PONG
			args := fieldsNoPrefix(line)
			target := *ss
			if len(args) > 0 {
				target = args[len(args)-1]
			}
			send(bw, fmt.Sprintf("%s Z %s", *ss, target))
		case "EB": // peer finished burst
			send(bw, fmt.Sprintf("%s EA", *ss))
		case "EA": // ack to our EB -> we're live
			if !linked {
				linked = true
				_ = c.SetDeadline(time.Time{}) // clear deadlines; we're linked now
				log.Printf("*** LINKED: newserv-q is live")

				// Auto-join channels and op Q
				if *introQ && qNumeric != "" {
					for _, ch := range autochans {
						ch = strings.TrimSpace(ch)
						if ch == "" {
							continue
						}
						// JOIN as the Q client: "<SSCCC> J #channel"
						send(bw, fmt.Sprintf("%s J %s", qNumeric, ch))
						// OPMODE from the server to op Q: "<SS> OM #channel +o <SSCCC>"
						send(bw, fmt.Sprintf("%s OM %s +o %s", *ss, ch, qNumeric))
					}
				}
			}
		case "P": // PRIVMSG to Q
			if *introQ {
				sender := srcPrefix(line) // user's numeric (UID)
				args := fieldsNoPrefix(line)
				if len(args) >= 2 {
					target := args[0] // should be qNumeric or Q's nick
					msg := restAfter(line, target)
					if target == qNumeric || strings.EqualFold(target, *nickQ) {
						cmdLine := strings.TrimSpace(strings.TrimPrefix(msg, ":"))
						processPM(bw, sender, cmdLine)
					}
				}
			}
		}

		// When not yet linked, keep a deadline so dead links time out
		if !linked {
			_ = c.SetDeadline(time.Now().Add(60 * time.Second))
		}
	}
}

func verb(line string) string {
	f := strings.Fields(line)
	if len(f) >= 2 {
		return f[1]
	}
	return ""
}

// ---- Command router (PM to Q) ----
func processPM(bw *bufio.Writer, senderNumeric, line string) {
	if line == "" {
		noticeAsQ(bw, senderNumeric, "Try: HELP")
		return
	}
	fields := strings.Fields(line)
	cmd := strings.ToUpper(fields[0])
	args := fields[1:]
	switch cmd {
	case "HELP":
		noticeAsQ(bw, senderNumeric, "Commands: REGISTER <account> <password> | LOGIN <account> <password> | LOGOUT | WHOAMI | HELP")
	case "REGISTER":
		if len(args) < 2 {
			noticeAsQ(bw, senderNumeric, "Usage: REGISTER <account> <password>")
			return
		}
		acc := strings.ToLower(args[0])
		pwd := strings.Join(args[1:], " ")
		if _, ok := db.Accounts[acc]; ok {
			noticeAsQ(bw, senderNumeric, "Account already exists")
			return
		}
		salt, err := newSalt()
		if err != nil {
			noticeAsQ(bw, senderNumeric, "Internal error")
			return
		}
		db.Accounts[acc] = Account{Name: acc, Salt: salt, Hash: hashPass(salt, pwd)}
		_ = db.save()

		identifyAndVhostP10(bw, senderNumeric, acc)
		noticeAsQ(bw, senderNumeric, "Account registered and identified as "+acc+". vHost set to "+vhostFor(acc))
	case "LOGIN":
		if len(args) < 2 {
			noticeAsQ(bw, senderNumeric, "Usage: LOGIN <account> <password>")
			return
		}
		acc := strings.ToLower(args[0])
		pwd := strings.Join(args[1:], " ")
		rec, ok := db.Accounts[acc]
		if !ok {
			noticeAsQ(bw, senderNumeric, "No such account")
			return
		}
		if hashPass(rec.Salt, pwd) != rec.Hash {
			noticeAsQ(bw, senderNumeric, "Invalid password")
			return
		}
		db.Sessions[senderNumeric] = acc
		_ = db.save()

		identifyAndVhostP10(bw, senderNumeric, acc)
		noticeAsQ(bw, senderNumeric, "Logged in as "+acc+" — vHost set to "+vhostFor(acc))
	case "LOGOUT":
		delete(db.Sessions, senderNumeric)
		_ = db.save()
		noticeAsQ(bw, senderNumeric, "Logged out")
	case "WHOAMI":
		if acc, ok := db.Sessions[senderNumeric]; ok {
			noticeAsQ(bw, senderNumeric, "You are "+acc)
		} else {
			noticeAsQ(bw, senderNumeric, "You are not logged in")
		}
	default:
		noticeAsQ(bw, senderNumeric, "Unknown command. Try: HELP")
	}
}

// Build vhost from pattern
func vhostFor(account string) string {
	return strings.ReplaceAll(*vhPattern, "{user}", account)
}

// ircu/snircd (P10): mark account login, set +r, ensure +x, and apply vhost via +h.
// ircu/snircd (P10): mark account login, set +r, ensure -x, then apply vhost via +h.
func identifyAndVhostP10(bw *bufio.Writer, uid, account string) {
	// 1) Account login -> WHOIS "is identified as ..."
	send(bw, fmt.Sprintf("%s SVSLOGIN %s %s", *ss, uid, account))
	// 2) Mark as identified
	send(bw, fmt.Sprintf("%s SVSMODE %s +r", *ss, uid))
	// 3) Make sure cloak is OFF so it won't override +h
	send(bw, fmt.Sprintf("%s SVS2MODE %s -x", *ss, uid))
	// 4) Apply the vhost
	vh := vhostFor(account)
	send(bw, fmt.Sprintf("%s SVS2MODE %s +h %s", *ss, uid, vh))
	log.Printf("identify/vhost applied: uid=%s account=%s modes=+r -x +h host=%s", uid, account, vh)
}

// Replies as the Q *client* (not as server)
func noticeAsQ(bw *bufio.Writer, targetNumeric, text string) {
	if qNumeric == "" { // fallback, should not happen
		noticeServer(bw, targetNumeric, text)
		return
	}
	send(bw, fmt.Sprintf("%s O %s :%s", qNumeric, targetNumeric, text))
}

// Server NOTICE (used only as fallback)
func noticeServer(bw *bufio.Writer, targetNumeric, text string) {
	send(bw, fmt.Sprintf("%s O %s :%s", *ss, targetNumeric, text))
}

func introduceQ(bw *bufio.Writer, now int64) {
	ccc := *fixedCCC
	if len(ccc) != 3 {
		ccc = b64(0, 3) // AAA for first run
	}
	ssccc := *ss + ccc
	qNumeric = ssccc  // remember Q's numeric
	ip64 := b64(0, 6) // 0.0.0.0
	line := fmt.Sprintf("%s N %s 1 %d %s %s +oi %s %s :%s", *ss, *nickQ, now, *userQ, *hostQ, ip64, ssccc, *realQ)
	send(bw, line)
}

func srcPrefix(line string) string {
	f := strings.Fields(line)
	if len(f) > 0 {
		return f[0]
	}
	return ""
}
func fieldsNoPrefix(line string) []string {
	f := strings.Fields(line)
	if len(f) <= 1 {
		return nil
	}
	return f[2:]
}
func restAfter(line, after string) string {
	idx := strings.Index(line, " "+after+" ")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(line[idx+len(after)+2:])
}

func send(bw *bufio.Writer, s string) {
	log.Printf("> %s", s)
	_, _ = bw.WriteString(s)
	_, _ = bw.WriteString("\r\n")
	_ = bw.Flush()
}
