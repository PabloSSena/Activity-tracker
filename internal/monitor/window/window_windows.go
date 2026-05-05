//go:build windows

package window

import (
	"context"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

func platformActiveWindow(_ context.Context) (title, proc string, err error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", "", nil
	}

	// Get window title
	buf := make([]uint16, 512)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	title = windows.UTF16ToString(buf)

	// Get PID
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return title, "", nil
	}

	// Get process name from handle
	h, err2 := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err2 != nil {
		return title, "", nil
	}
	defer windows.CloseHandle(h)

	exeBuf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(exeBuf))
	if err2 := windows.QueryFullProcessImageName(h, 0, &exeBuf[0], &size); err2 != nil {
		return title, "", nil
	}
	proc = filepath.Base(windows.UTF16ToString(exeBuf[:size]))
	return title, proc, nil
}
