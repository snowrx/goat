package main

import (
	"io"
	"log"
	"net"
	"net/netip"
	"sync"
)

const (
	LISTEN_PORT = ":40960"
	BUFFER_SIZE = 1 << 20
)

func main() {
	lnAddr, err := net.ResolveTCPAddr("tcp", LISTEN_PORT)
	if err != nil {
		log.Fatalf("%s", err)
		return
	}

	ln, err := net.ListenTCP("tcp", lnAddr)
	if err != nil {
		log.Fatalf("%s", err)
		return
	}

	err = handleListener(ln)
	if err != nil {
		log.Fatalf("%s", err)
		return
	}
}

func handleListener(ln *net.TCPListener) error {
	defer ln.Close()

	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			return err
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn *net.TCPConn) {
	defer conn.Close()

	clientAP, err := netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}

	localAP, err := netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}

	targetAP, err := GetOriginalDst(conn)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}

	if *targetAP == localAP {
		log.Printf("REJECTED: %s", clientAP)
		return
	}

	proxyConn, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(*targetAP))
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}
	defer proxyConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go relay(&wg, conn, proxyConn)
	go relay(&wg, proxyConn, conn)
	log.Printf("OPENED: %50s <-> %50s", clientAP, targetAP)
	wg.Wait()
	log.Printf("CLOSED: %50s <-> %50s", clientAP, targetAP)
}

func relay(wg *sync.WaitGroup, src *net.TCPConn, dst *net.TCPConn) {
	defer dst.Close()
	defer wg.Done()

	_, err := io.CopyBuffer(dst, src, make([]byte, BUFFER_SIZE))
	if err != nil {
		log.Printf("ERROR: %s", err)
	}
}
