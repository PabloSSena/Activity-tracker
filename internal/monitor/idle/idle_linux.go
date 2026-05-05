//go:build linux

package idle

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type platformDetector struct{}

// New returns the platform idle detector.
func New() Detector { return &platformDetector{} }

func (p *platformDetector) IdleDuration() time.Duration {
	out, err := exec.Command("xprintidle").Output()
	if err != nil {
		return 0
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
