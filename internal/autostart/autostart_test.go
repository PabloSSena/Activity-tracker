package autostart_test

import (
	"testing"

	"github.com/user/activitytracker/internal/autostart"
)

func TestAutostart_EnableDisable(t *testing.T) {
	a := autostart.New("activitytracker-test", "/tmp/activitytracker-test")

	// Clean up before and after
	_ = a.Disable()
	defer a.Disable()

	if err := a.Enable(); err != nil {
		t.Fatalf("Enable() error: %v", err)
	}
	if !a.IsEnabled() {
		t.Fatal("IsEnabled() = false after Enable()")
	}

	if err := a.Disable(); err != nil {
		t.Fatalf("Disable() error: %v", err)
	}
	if a.IsEnabled() {
		t.Fatal("IsEnabled() = true after Disable()")
	}
}

func TestAutostart_EnableIdempotent(t *testing.T) {
	a := autostart.New("activitytracker-test", "/tmp/activitytracker-test")
	defer a.Disable()

	if err := a.Enable(); err != nil {
		t.Fatalf("first Enable() error: %v", err)
	}
	if err := a.Enable(); err != nil {
		t.Fatalf("second Enable() error: %v", err)
	}
	if !a.IsEnabled() {
		t.Fatal("IsEnabled() = false after double Enable()")
	}
}
