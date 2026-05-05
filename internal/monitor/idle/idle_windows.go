//go:build windows

package idle

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procGetLastInputInfo = user32.NewProc("GetLastInputInfo")

	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount = kernel32.NewProc("GetTickCount")
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

type platformDetector struct{}

// New returns the platform idle detector.
func New() Detector { return &platformDetector{} }

func (p *platformDetector) IdleDuration() time.Duration {
	var info lastInputInfo
	info.cbSize = uint32(unsafe.Sizeof(info))
	ret, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return 0
	}
	tickNow, _, _ := procGetTickCount.Call()
	idleMs := uint32(tickNow) - info.dwTime
	return time.Duration(idleMs) * time.Millisecond
}
