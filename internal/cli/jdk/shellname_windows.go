//go:build windows

package jdkcli

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func parentExeName() string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getppid()))
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
