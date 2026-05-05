package idle

import "time"

// Detector reports how long the user has been idle (no keyboard/mouse input).
type Detector interface {
	IdleDuration() time.Duration
}
