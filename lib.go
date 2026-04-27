package main

import (
	"encoding/binary"
	"net"
	"net/netip"
	"syscall"
	"unsafe"
)

const SO_ORIGINAL_DST = 80

func GetOriginalDst(conn *net.TCPConn) (*netip.AddrPort, error) {
	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
	v4 := remoteAddr.IP.To4()

	rawConn, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var res netip.AddrPort
	var opErr error

	err = rawConn.Control(func(fd uintptr) {
		// v4 or mapped
		if v4 != nil {
			var raw syscall.RawSockaddrInet4
			size := uint32(unsafe.Sizeof(raw))
			_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd,
				syscall.SOL_IP, SO_ORIGINAL_DST,
				uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&size)), 0)
			if errno != 0 {
				opErr = errno
				return
			}
			res = netip.AddrPortFrom(netip.AddrFrom4(raw.Addr), ntohs(raw.Port))
		} else {
			// v6
			var raw syscall.RawSockaddrInet6
			size := uint32(unsafe.Sizeof(raw))
			_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd,
				syscall.SOL_IPV6, SO_ORIGINAL_DST,
				uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&size)), 0)
			if errno != 0 {
				opErr = errno
				return
			}
			res = netip.AddrPortFrom(netip.AddrFrom16(raw.Addr), ntohs(raw.Port))
		}
	})

	if err != nil {
		return nil, err
	}

	if opErr != nil {
		return nil, opErr
	}

	return &res, nil
}

func ntohs(p uint16) uint16 {
	return binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&p))[:])
}
