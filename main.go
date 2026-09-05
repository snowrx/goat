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
const DEBUG = true

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
	accepted := time.Now()

	clientAP, cl_parse_err := netip.ParseAddrPort(conn.RemoteAddr().String())
	localAP, lo_parse_err := netip.ParseAddrPort(conn.LocalAddr().String())
	targetAP, tg_parse_err := GetOriginalDst(conn)
	if cl_parse_err != nil || lo_parse_err != nil || tg_parse_err != nil {
		logger("ERROR", "Failed to retrieve connection information")
		return
	}

	label := fmt.Sprintf("%50s <> %50s", clientAP, targetAP)
	parsed := time.Now()

	if *targetAP == localAP {
		logger("REJECT", label)
		return
	}

	buf := make([]byte, TFO_SIZE)
	conn.SetReadDeadline(time.Now().Add(TFO_WAIT_MS * time.Millisecond))
	n, _ := conn.Read(buf)
	conn.SetReadDeadline(time.Time{})
	buf = buf[:n]
	received := time.Now()

	proxyConn, dial_err := tfo.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(*targetAP), buf)
	if dial_err != nil {
		logger("ERROR", dial_err.Error())
		return
	}
	defer proxyConn.Close()
	opened := time.Now()

	logger("OPEN", label)
	if DEBUG {
		total_time := opened.Sub(accepted).Microseconds()
		resolve_time := parsed.Sub(accepted).Microseconds()
		receive_time := received.Sub(parsed).Microseconds()
		open_time := opened.Sub(received).Microseconds()
		logger("DEBUG", fmt.Sprintf("time: %6d us (res: %3d / recv: %4d / open: %6d) / data: %4d B", total_time, resolve_time, receive_time, open_time, n))
	}
	proxyConn.SetDeadline(time.Now().Add(CONN_LIFETIME))
	conn.SetDeadline(time.Now().Add(CONN_LIFETIME))
	relay(conn, proxyConn)
	logger("CLOSE", label)
}

func relay(client, upstream net.Conn) {
	var wg sync.WaitGroup

	wg.Go(func() {
		// client -> upstream
		if _, err := io.Copy(upstream, client); err != nil {
			logger("ERROR", "copy error in upload")
			client.Close()
			upstream.Close()
			return
		}
		if err := halfCloseWrite(upstream); err != nil {
			logger("ERROR", "close error in upload")
		}
	})
	wg.Go(func() {
		// upstream -> client
		if _, err := io.Copy(client, upstream); err != nil {
			logger("ERROR", "copy error in download")
			upstream.Close()
			client.Close()
			return
		}
		if err := halfCloseWrite(client); err != nil {
			logger("ERROR", "close error in download")
		}
	})

	wg.Wait()
}

func halfCloseWrite(conn net.Conn) error {
	if tc, ok := conn.(*net.TCPConn); ok {
		return tc.CloseWrite()
	}
	return nil
}

func logger(subject string, message string) {
	log.Printf("| %-10s | %s", subject, message)
}
