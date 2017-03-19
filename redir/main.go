package main

import (
	"io"
	"net"
	"syscall"
	"fmt"
	"os"
	"unsafe"
	"time"
	"sync"
)

func checkErr(err error, msg string) {
	if err == nil {
		return
	}
	fmt.Println(err.Error(), ":", msg)
	os.Exit(-1)
}

func setIpTransparent(listener *net.TCPListener) error {
	file, err := listener.File()
	if err != nil {
		return err
	}
	defer file.Close()

	err = syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	if err != nil {
		return err
	}

	return nil
}

func getOriginalDestTproxy(conn *net.TCPConn) (*net.TCPAddr, error) {
	file, err := conn.File()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var addr syscall.RawSockaddrAny
	var addrlen int = syscall.SizeofSockaddrAny
	_, _, e1 := syscall.RawSyscall(syscall.SYS_GETSOCKNAME, file.Fd(),
		uintptr(unsafe.Pointer(&addr)), uintptr(unsafe.Pointer(&addrlen)))
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


func getOriginalDestRedir(conn *net.TCPConn) (*net.TCPAddr, error) {
	file, err := conn.File()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var addr syscall.RawSockaddrAny
	var addrlen int = syscall.SizeofSockaddrAny
	_, _, e1 := syscall.Syscall6(syscall.SYS_GETSOCKOPT, file.Fd(), syscall.SOL_IP, 80,
		uintptr(unsafe.Pointer(&addr)), uintptr(unsafe.Pointer(&addrlen)), 0)
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

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024)
	},
}

func connFwd(a, b *net.TCPConn, die chan int) {
	io.Copy(a, b)
	die <- 1
	return
}

func forwardConn(origConn *net.TCPConn) {
	dst, err := getOriginalDestRedir(origConn)
	if err != nil {
		dst, err = getOriginalDestTproxy(origConn)
		if err != nil {
			fmt.Println("Unable to get original dest for", origConn.RemoteAddr())
			origConn.Close()
			return
		}
	}

	dstStr := dst.String()
	fmt.Println("Accept connection from", origConn.RemoteAddr(), "to", dstStr)

	fwdConn_, err := net.DialTimeout("tcp4", dstStr, 5 * time.Second)
	if err != nil {
		fmt.Println("Cannot connect to", dst)
		origConn.Close()
		return
	}
	fwdConn := fwdConn_.(*net.TCPConn)

	p1die := make(chan int)
	go connFwd(origConn, fwdConn, p1die)
	p2die := make(chan int)
	go connFwd(fwdConn, origConn, p2die)

	select {
	case <-p1die:
	case <-p2die:
	}

	origConn.Close()
	fwdConn.Close()

	fmt.Println("Close connection from", origConn.RemoteAddr(), "to", dstStr)
}

func main() {
	addr, _ := net.ResolveTCPAddr("tcp4", ":" + os.Args[1])
	listener, err := net.ListenTCP("tcp4", addr)
	checkErr(err, "listen")

	err = setIpTransparent(listener)
	checkErr(err, "set IP_TRANSPARENT")

	for {
		conn, err := listener.AcceptTCP()
		checkErr(err, "accept TCP")
		go forwardConn(conn)
	}

}
