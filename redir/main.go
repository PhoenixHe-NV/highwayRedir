package main

import (
	"net"
	"syscall"
	"fmt"
	"os"
	"unsafe"
	"io"
)

func checkErr(err error, msg string) {
	if err == nil {
		return
	}
	fmt.Println(err.Error(), ":", msg)
	os.Exit(-1)
}

func getOriginalDest(conn *net.TCPConn) (*net.TCPAddr, error) {
	file, err := conn.File()
	if err != nil {
		return nil, err
	}

	var addr syscall.RawSockaddrAny
	var addr_len int = syscall.SizeofSockaddrAny
	_, _, e1 := syscall.Syscall6(syscall.SYS_GETSOCKOPT, file.Fd(), syscall.SOL_IP, 80,
		uintptr(unsafe.Pointer(&addr)), uintptr(unsafe.Pointer(&addr_len)), 0)
	if e1 != 0 {
		return nil, e1
	}

	pp := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&addr))
	p := (*[2]byte)(unsafe.Pointer(&pp.Port))
	port := int(p[0])<<8 + int(p[1])

	ret := net.TCPAddr{make([]byte, 4), port, ""}
	ret.IP[0] = pp.Addr[0]
	ret.IP[1] = pp.Addr[1]
	ret.IP[2] = pp.Addr[2]
	ret.IP[3] = pp.Addr[3]

	return &ret, nil
}

func connFwd(a, b *net.TCPConn) {
	io.Copy(a, b)
	a.Close()
	b.Close()
}

func forwardConn(origConn *net.TCPConn, dst *net.TCPAddr) {
	fwdConn, err := net.DialTCP("tcp4", nil, dst)
	if err != nil {
		fmt.Println("Cannot connect to", dst)
		origConn.Close()
		return
	}

	go connFwd(origConn, fwdConn)
	connFwd(fwdConn, origConn)
	fmt.Println("Close connection from", origConn.RemoteAddr(), "to", dst)
}

func main() {
	addr, _ := net.ResolveTCPAddr("tcp4", ":" + os.Args[1])
	listener, err := net.ListenTCP("tcp4", addr)
	checkErr(err, "listen")

	for {
		conn, err := listener.AcceptTCP()
		checkErr(err, "accept TCP")

		addr, err := getOriginalDest(conn)
		if err != nil {
			fmt.Println("Unable to get original dest for", conn.RemoteAddr())
			conn.Close()
			continue
		}

		fmt.Println("Accept connection from", conn.RemoteAddr(), "to", addr)
		go forwardConn(conn, addr)
	}

}