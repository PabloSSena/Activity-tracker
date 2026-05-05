//go:build linux

package window

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func platformActiveWindow(ctx context.Context) (title, proc string, err error) {
	// Get active window title
	out, err := exec.CommandContext(ctx, "xdotool", "getactivewindow", "getwindowname").Output()
	if err != nil {
		return "", "", fmt.Errorf("window: xdotool getwindowname: %w", err)
	}
	title = strings.TrimSpace(string(out))

	// Get PID of active window
	pidOut, err := exec.CommandContext(ctx, "xdotool", "getactivewindow", "getwindowpid").Output()
	if err != nil {
		return title, "", nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidOut)))
	if err != nil {
		return title, "", nil
	}

	// Read process name from /proc/<pid>/comm
	commPath := filepath.Join("/proc", strconv.Itoa(pid), "comm")
	commBytes, err := os.ReadFile(commPath)
	if err != nil {
		return title, "", nil
	}
	proc = strings.TrimSpace(string(commBytes))
	return title, proc, nil
}
