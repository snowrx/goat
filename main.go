package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"sync"
)

const LISTEN_PORT = ":40960"

func main() {
	lnAddr, err := net.ResolveTCPAddr("tcp", LISTEN_PORT)
	if err != nil {
		log.Fatalf("Failed to resolve listener endpoint: %s", err)
	}

	ln, err := net.ListenTCP("tcp", lnAddr)
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
	label_up := fmt.Sprintf("%50s -> %50s", clientAP, targetAP)
	label_down := fmt.Sprintf("%50s <- %50s", clientAP, targetAP)

	if *targetAP == localAP {
		logger("REJECT", label_up)
		return
	}

	proxyConn, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(*targetAP))
	if err != nil {
		logger("ERROR", err.Error())
		return
	}
	defer proxyConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go relay(&wg, label_up, conn, proxyConn)
	go relay(&wg, label_down, proxyConn, conn)
	logger("OPEN", label_up)
	wg.Wait()
	logger("CLOSE", label_up)
}

func relay(wg *sync.WaitGroup, label string, src *net.TCPConn, dst *net.TCPConn) {
	defer wg.Done()
	defer src.CloseRead()
	defer dst.CloseWrite()

	_, err := io.Copy(dst, src)
	if err != nil {
		logger("ERROR", label)
	}
}

func logger(subject string, message string) {
	log.Printf("| %10s | %s", subject, message)
}
