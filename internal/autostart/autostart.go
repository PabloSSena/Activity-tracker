package autostart

// Starter manages OS login auto-start registration.
type Starter interface {
	Enable() error
	Disable() error
	IsEnabled() bool
}

// App is the concrete autostart manager.
type App struct {
	name    string // display name / registry key name
	exePath string // full path to the binary
}

// New creates an autostart manager for the given app name and binary path.
func New(name, exePath string) *App {
	return &App{name: name, exePath: exePath}
}
