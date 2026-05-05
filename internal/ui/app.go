package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"

	"fyne.io/systray"
)

// App manages the system tray icon and menu.
type App struct {
	srv *ReportServer
}

// New creates an App backed by the given ReportServer.
func New(srv *ReportServer) *App {
	return &App{srv: srv}
}

// Run starts the system tray event loop (blocks until the user chooses Quit).
func (a *App) Run(onQuit func()) {
	systray.Run(a.ready(onQuit), func() {})
}

func (a *App) ready(onQuit func()) func() {
	return func() {
		systray.SetIcon(trayIcon())
		systray.SetTooltip("Activity Tracker")

		mOpen := systray.AddMenuItem("Open Report", "Open daily report in browser")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop activity tracking")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					if err := a.srv.OpenInBrowser(); err != nil {
						log.Printf("ui: open browser: %v", err)
					}
				case <-mQuit.ClickedCh:
					onQuit()
					systray.Quit()
					return
				}
			}
		}()
	}
}

// trayIcon returns a minimal 16×16 blue PNG byte slice.
func trayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	blue := color.RGBA{R: 30, G: 100, B: 200, A: 255}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, blue)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
