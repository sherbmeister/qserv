// qserv_link.go
package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func sendLine(conn net.Conn, line string) {
	fmt.Fprintf(conn, "%s\r\n", line)
	fmt.Println(">>>", line)
}

func main() {
	// Configuration
	serverName := "qserv.i-tna.org"
	host := "127.0.0.1:7029"
	// password := "abc123"
	mySid := "42S" // 3-character server ID (must be unique on the network)
	qUID := mySid + "AAAAAA" // 9-char UID: 3-char SID + 6-char user ID
	now := time.Now().Unix()
	ts := fmt.Sprintf("%d", now)

	// Connect
	conn, err := net.Dial("tcp", host)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// TS6 Handshake
    sendLine(conn, "PASS qtest123 TS 6 :qserv.i-tna.org")
    sendLine(conn, "SERVER qserv.i-tna.org 1 :EmechNET Services")
	sendLine(conn, fmt.Sprintf("SVINFO 6 6 0 :%s", ts))

	// Inject Q
	nick := "Q"
	hostname := "qserv.i-tna.org"
	ident := "qserv"
	realname := "EmechNET Q Service"
	modes := "+oS" // IRC operator + service flag
	sendLine(conn, fmt.Sprintf("UID %s 1 %s %s %s %s %s %s :%s",
		qUID, // Unique ID
		ts,    // Timestamp
		nick,
		modes,
		ident,
		hostname,
		serverName,
		realname,
	))

	// Idle loop to keep the connection alive
	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Disconnected:", err)
			return
		}
		line := strings.TrimSpace(string(buf[:n]))
		fmt.Println("<<<", line)
	}
}

