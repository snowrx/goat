package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/database64128/tfo-go/v2"
)

const LISTEN_PORT = ":40960"
const CONN_LIFETIME = 24 * time.Hour
const TFO_SIZE = 1 << 12
const TFO_WAIT_MS = 4

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

	buf := make([]byte, TFO_SIZE)
	conn.SetReadDeadline(time.Now().Add(TFO_WAIT_MS * time.Millisecond))
	n, err := conn.Read(buf)
	conn.SetReadDeadline(time.Time{})
	buf = buf[:n]

	start := time.Now()
	proxyConn, err := tfo.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(*targetAP), buf)
	if err != nil {
		logger("ERROR", err.Error())
		return
	}
	defer proxyConn.Close()

	logger("OPEN", label)
	logger(fmt.Sprintf("%4d/%4d", time.Since(start).Milliseconds(), n), label)
	proxyConn.SetDeadline(time.Now().Add(CONN_LIFETIME))
	conn.SetDeadline(time.Now().Add(CONN_LIFETIME))
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
	log.Printf("| %-10s | %s", subject, message)
}
