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
	BUFFER_SIZE = 1 << 24
)

func main() {
	lnAddr, err := net.ResolveTCPAddr("tcp", LISTEN_PORT)
	if err != nil {
		log.Fatalln("Failed to parse listen addr")
		return
	}

	ln, err := net.ListenTCP("tcp", lnAddr)
	if err != nil {
		log.Fatalln("Failed to open listener")
		return
	}

	err = handleListener(ln)
	if err != nil {
		log.Fatalln("Failed to handle listener")
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
		log.Printf("Failed to parse client addr: %s", err)
		return
	}

	localAP, err := netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		log.Printf("Failed to parse local addr: %s", err)
		return
	}

	targetAP, err := GetOriginalDst(conn)
	if err != nil {
		log.Printf("Failed to get original dst: %s", err)
		return
	}

	if *targetAP == localAP {
		log.Printf("Rejected direct connection from %s", clientAP)
		return
	}

	proxyConn, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(*targetAP))
	if err != nil {
		log.Printf("Failed to dial proxy: %s", err)
		return
	}
	defer proxyConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go relay(&wg, conn, proxyConn)
	go relay(&wg, proxyConn, conn)
	log.Printf("Relaying %s <-> %s", clientAP, targetAP)
	wg.Wait()
	log.Printf("Relay closed %s <-> %s", clientAP, targetAP)
}

func relay(wg *sync.WaitGroup, src *net.TCPConn, dst *net.TCPConn) {
	defer dst.Close()
	defer wg.Done()
	_, err := io.CopyBuffer(dst, src, make([]byte, BUFFER_SIZE))
	if err != nil {
		log.Printf("Failed to copy: %s", err)
	}
}
