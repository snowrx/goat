package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/sagernet/tfo-go"
)

const LISTEN_PORT = ":40960"
const BUF_SIZE = 1 << 12

func main() {
	lnAddr, err := net.ResolveTCPAddr("tcp", LISTEN_PORT)
	if err != nil {
		log.Fatalf("Failed to resolve listener endpoint: %s", err)
	}

	ln, err := tfo.ListenTCP("tcp", lnAddr)
	if err != nil {
		log.Fatalf("Failed to create listener: %s", err)
	}
	defer ln.Close()
	log.Printf("Proxy started on %s", LISTEN_PORT)

	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			log.Printf("Failed to accept connection: %s", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn *net.TCPConn) {
	defer conn.Close()

	// prepare
	buf := make([]byte, BUF_SIZE)

	clientAP, err := netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		logger("ERROR", err.Error())
		return
	}

	localAP, err := netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		logger("ERROR", err.Error())
		return
	}

	targetAP, err := GetOriginalDst(conn)
	if err != nil {
		logger("ERROR", err.Error())
		return
	}
	label := fmt.Sprintf("%50s <> %50s", clientAP, targetAP)

	if *targetAP == localAP {
		logger("REJECT", label)
		return
	}

	// tfo
	conn.SetReadDeadline(time.Now().Add(2 * time.Millisecond))
	n, _ := conn.Read(buf)
	conn.SetReadDeadline(time.Time{})
	firstBytes := buf[:n]
	if n > 0 {
		logger(fmt.Sprintf("TFO:%4d", n), label)
	}

	proxyConn, err := tfo.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(*targetAP), firstBytes)
	if err != nil {
		logger("ERROR", err.Error())
		return
	}
	defer proxyConn.Close()

	// run
	logger("OPEN", label)
	relay(conn, proxyConn)
	logger("CLOSE", label)
}

func relay(client, upstream net.Conn) {
	var wg sync.WaitGroup

	wg.Go(func() {
		io.Copy(upstream, client)
		halfCloseWrite(upstream)
	})

	wg.Go(func() {
		io.Copy(client, upstream)
		halfCloseWrite(client)
	})

	wg.Wait()
}

func halfCloseWrite(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
}

func logger(subject string, message string) {
	log.Printf("| %10s | %s", subject, message)
}
