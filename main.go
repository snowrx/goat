package main

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/database64128/tfo-go/v2"
)

const DEBUG bool = true

const LISTEN_PORT string = ":40960"
const CONN_LIFETIME time.Duration = 4 * time.Hour
const TFO_SIZE int = 1 << 15
const TFO_WAIT_MS int = 4
const NODELAY bool = true

const BURST_LIMIT int64 = 1 << 25

func main() {
	lnAddr, err := net.ResolveTCPAddr("tcp", LISTEN_PORT)
	if err != nil {
		fmt.Printf("Failed to resolve listener endpoint: %s\n", err)
	}

	ln, err := tfo.ListenTCP("tcp", lnAddr)
	if err != nil {
		fmt.Printf("Failed to create listener: %s\n", err)
	}
	defer ln.Close()
	fmt.Printf("Proxy started on %s\n", LISTEN_PORT)

	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			fmt.Printf("Failed to accept connection: %s\n", err)
			continue
		}
		conn.SetNoDelay(NODELAY)
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
	conn.SetReadDeadline(time.Now().Add(time.Duration(TFO_WAIT_MS) * time.Millisecond))
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
	proxyConn.SetNoDelay(NODELAY)
	opened := time.Now()

	logger("OPEN", label)
	if DEBUG {
		total_time := opened.Sub(accepted).Microseconds()
		resolve_time := parsed.Sub(accepted).Microseconds()
		receive_time := received.Sub(parsed).Microseconds()
		open_time := opened.Sub(received).Microseconds()
		logger("DEBUG", fmt.Sprintf("time: %7d us (res: %3d / recv: %4d / open: %7d) / data: %5d B", total_time, resolve_time, receive_time, open_time, n))
	}
	proxyConn.SetDeadline(time.Now().Add(CONN_LIFETIME))
	conn.SetDeadline(time.Now().Add(CONN_LIFETIME))
	relay(conn, proxyConn)
	logger("CLOSE", label)
	duration := time.Since(opened)
	if DEBUG {
		logger("DEBUG", fmt.Sprintf("total time: %7.2fs", duration.Seconds()))
	}
}

func relay(client, upstream net.Conn) {
	var wg sync.WaitGroup

	wg.Go(func() {
		// client -> upstream
		if _, err := copy(upstream, client); err != nil {
			logger("ERROR", fmt.Sprintf("copy error in upload: %s", err))
			abortConn(client, upstream)
			return
		}
		if err := halfCloseWrite(upstream); err != nil {
			logger("ERROR", fmt.Sprintf("copy error in upload: %s", err))
		}
	})
	wg.Go(func() {
		// upstream -> client
		if _, err := copy(client, upstream); err != nil {
			logger("ERROR", fmt.Sprintf("copy error in download: %s", err))
			abortConn(client, upstream)
			return
		}
		if err := halfCloseWrite(client); err != nil {
			logger("ERROR", fmt.Sprintf("copy error in download: %s", err))
		}
	})

	wg.Wait()
}

func copy(dst, src net.Conn) (written int64, err error) {
	// return io.Copy(dst, src)
	var p int64
	var e error

	for true {
		p, e = io.CopyN(dst, src, BURST_LIMIT)

		if p == 0 || e != nil {
			break
		}

		written += p
		time.Sleep(time.Millisecond)
	}

	if e != nil && e != io.EOF {
		err = e
	}

	return written, err
}

func halfCloseWrite(conn net.Conn) error {
	if tc, ok := conn.(*net.TCPConn); ok {
		return tc.CloseWrite()
	}
	return nil
}

func abortConn(c1, c2 net.Conn) {
	if tc, ok := c1.(*net.TCPConn); ok {
		tc.SetLinger(0)
	}
	if tc, ok := c2.(*net.TCPConn); ok {
		tc.SetLinger(0)
	}
	c1.Close()
	c2.Close()
}

func logger(subject string, message string) {
	fmt.Printf("%10s | %s\n", subject, message)
}
