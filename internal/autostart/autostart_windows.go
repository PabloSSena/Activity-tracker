//go:build windows

package autostart

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func (a *App) Enable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("autostart: open registry key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(a.name, a.exePath); err != nil {
		return fmt.Errorf("autostart: set registry value: %w", err)
	}
	return nil
}

func (a *App) Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return nil // key missing = already disabled
	}
	defer k.Close()
	_ = k.DeleteValue(a.name)
	return nil
}

func (a *App) IsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	val, _, err := k.GetStringValue(a.name)
	return err == nil && val == a.exePath
}
