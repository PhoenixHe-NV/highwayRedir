package main

import (
	"runtime"
	"io"
	"net"
	"syscall"
	"fmt"
	"os"
	"unsafe"
	"sync"
	"time"
)

func checkErr(err error, msg string) {
	if err == nil {
		return
	}
	fmt.Println(err.Error(), ":", msg)
	os.Exit(-1)
}

func setIpTransparent(fd uintptr) error {
	err := syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	if err != nil {
		return err
	}

	return nil
}

func getOriginalDestTproxy(fd uintptr) (*net.TCPAddr, error) {
	var addr syscall.RawSockaddrAny
	var addrlen int = syscall.SizeofSockaddrAny
	_, _, e1 := syscall.RawSyscall(syscall.SYS_GETSOCKNAME, fd,
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


func getOriginalDestRedir(fd uintptr) (*net.TCPAddr, error) {
	var addr syscall.RawSockaddrAny
	var addrlen int = syscall.SizeofSockaddrAny
	_, _, e1 := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, syscall.SOL_IP, 80,
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

func connFwd(src, dst net.Conn) {
	defer src.Close()
	defer dst.Close()

	buf := bufPool.Get().([]byte)
	defer bufPool.Put(buf)

	for {
		src.SetDeadline(time.Now().Add(30 * time.Second))
		nr, er := src.Read(buf)

		if nr > 0 {
			dst.SetDeadline(time.Now().Add(5 * time.Second))
			nw, ew := dst.Write(buf[0:nr])
			if ew != nil || nr != nw {
				break
			}
		}
		if er == io.EOF || er != nil {
			break
		}
	}
}

func forwardConn(origConn *net.TCPConn) {
	origStr := origConn.RemoteAddr().String()

	file, err := origConn.File()
	origConn.Close()
	if err != nil {
		fmt.Println("Unable to dup fd for", origStr)
		return
	}

	fd := file.Fd()
	dst, err := getOriginalDestTproxy(fd)
	if err != nil {
		fmt.Println("Unable to get original dest for", origStr)
		file.Close()
		return
	}

	conn, err := net.FileConn(file)
	if err != nil {
		fmt.Println("Unable to dup new origConn")
		file.Close()
		return
	}

	dstStr := dst.String()
	fmt.Println("Accept connection from", origStr, "to", dstStr, "dupFd:", fd)
	file.Close()

	fwdConn, err := net.DialTCP("tcp4", nil, dst)
	if err != nil {
		fmt.Println("Cannot connect to", dst)
		conn.Close()
		return
	}

	go connFwd(conn, fwdConn)
	connFwd(fwdConn, conn)

	fmt.Println("Close connection from", origStr, "to", dstStr)
}

func main() {
	fmt.Println("runtime.NumCPU():", runtime.NumCPU())
	runtime.GOMAXPROCS(runtime.NumCPU())

	addr, _ := net.ResolveTCPAddr("tcp4", ":" + os.Args[1])
	listener, err := net.ListenTCP("tcp4", addr)
	checkErr(err, "listen")

	file, err := listener.File()
	err = setIpTransparent(file.Fd())
	checkErr(err, "set IP_TRANSPARENT")
	file.Close()

	for {
		conn, err := listener.AcceptTCP()
		checkErr(err, "accept TCP")
		go forwardConn(conn)
	}

}
