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
	serverName := "services.emechnet.org"
	host := "127.0.0.1:7029"
	// password := "password123"
	mySid := "042"
	qUID := mySid + "AAAAAA"
	ts := fmt.Sprintf("%d", time.Now().Unix())

	// Connect
	conn, err := net.Dial("tcp", host)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// TS6 Handshake - correct UnrealIRCd 6 style
now := time.Now().Unix()

sendLine(conn, "PASS password123 TS 6 :services.emechnet.org")
sendLine(conn, "PROTOCTL EAUTH=services.emechnet.org SID=042")
sendLine(conn, "SERVER services.emechnet.org 1 :EmechNET Q Service")
sendLine(conn, fmt.Sprintf("SVINFO 6 6 0 :%d", now))
sendLine(conn, fmt.Sprintf("SID services.emechnet.org 1 %d :042", now))


	// Inject Q user
	nick := "Q"
	ident := "qserv"
	realname := "EmechNET Services"
	modes := "+oS"
	sendLine(conn, fmt.Sprintf("UID %s 1 %s %s %s %s %s %s :%s",
		qUID, ts, nick, modes, ident, serverName, serverName, realname,
	))

	// Keep connection open
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
