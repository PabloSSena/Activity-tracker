//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func desktopPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart", name+".desktop"), nil
}

func (a *App) Enable() error {
	path, err := desktopPath(a.name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("autostart: create autostart dir: %w", err)
	}
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nHidden=false\nNoDisplay=false\nX-GNOME-Autostart-enabled=true\n",
		a.name, a.exePath)
	return os.WriteFile(path, []byte(content), 0o644)
}

func (a *App) Disable() error {
	path, err := desktopPath(a.name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("autostart: remove desktop file: %w", err)
	}
	return nil
}

func (a *App) IsEnabled() bool {
	path, err := desktopPath(a.name)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), a.exePath)
}
