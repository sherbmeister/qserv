package main

import (
	"fmt"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:7029")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	passLine := "PASS qtest123 TS 6 :qserv.i-tna.org\r\n"
	fmt.Printf("Sending: %q\n", passLine)
	fmt.Printf("Raw bytes: %v\n", []byte(passLine))

	_, err = conn.Write([]byte(passLine))
	if err != nil {
		panic(err)
	}

	// Wait for server reply
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Disconnected:", err)
		return
	}
	fmt.Println("Server replied:", string(buf[:n]))
}

