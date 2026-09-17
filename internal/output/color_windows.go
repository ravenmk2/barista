//go:build windows

package output

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const fileNameInfo = 2

var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode               = kernel32.NewProc("GetConsoleMode")
	procGetFileInformationByHandleEx = kernel32.NewProc("GetFileInformationByHandleEx")
)

func stdoutIsTerminal(f *os.File) bool {
	var st uint32
	r, _, _ := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&st)))
	if r != 0 {
		return true
	}
	return isPtyPipe(f)
}

func isPtyPipe(f *os.File) bool {
	var buf [520]byte
	r, _, _ := procGetFileInformationByHandleEx.Call(
		uintptr(syscall.Handle(f.Fd())),
		uintptr(fileNameInfo),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r == 0 {
		return false
	}
	n := *(*uint32)(unsafe.Pointer(&buf[0]))
	if n == 0 || int(n) > len(buf)-4 {
		return false
	}
	name := syscall.UTF16ToString((*[258]uint16)(unsafe.Pointer(&buf[4]))[: n/2 : n/2])
	return isCygwinPtyName(name)
}

func isCygwinPtyName(name string) bool {
	token := strings.Split(name, "-")
	if len(token) < 5 {
		return false
	}
	if token[0] != `\msys` && token[0] != `\cygwin` &&
		token[0] != `\Device\NamedPipe\msys` && token[0] != `\Device\NamedPipe\cygwin` {
		return false
	}
	if !isHexToken(token[1]) {
		return false
	}
	if !strings.HasPrefix(token[2], "pty") {
		return false
	}
	if token[3] != "from" && token[3] != "to" {
		return false
	}
	if token[4] != "master" {
		return false
	}
	return true
}

func isHexToken(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
