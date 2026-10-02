package main

import (
	"embed"
	"flag"
	"log"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:frontend/dist
var assets embed.FS

const eventStreamName = "poc:event-stream"

func main() {
	runtimeRootFlag := flag.String("runtime-root", "", "AgentDock runtime root override")
	flag.Parse()

	settings := NewSettingsService("")
	prefs := settings.Get().Preferences
	runtimeService := NewRuntimeService(*runtimeRootFlag)
	eventService := NewEventService()

	app := application.New(application.Options{
		Name:        "AgentDock Shared Desktop POC",
		Description: "Cross-platform AgentDock desktop architecture spike",
		Services: []application.Service{
			application.NewService(runtimeService),
			application.NewService(settings),
			application.NewService(eventService),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: securityHeadersMiddleware,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	app.HandleStream(eventStreamName, eventService.serveStream)

	width, height := normaliseWindowSize(prefs.WindowWidth, prefs.WindowHeight)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:               "agentdock-shared-poc",
		Title:              "AgentDock Shared Desktop POC",
		Width:              width,
		Height:             height,
		MinWidth:           760,
		MinHeight:          560,
		UseApplicationMenu: true,
		BackgroundColour:   application.NewRGB(19, 21, 27),
		URL:                "/",
	})

	showWindow := func() {
		window.Show()
		window.Restore()
		window.Focus()
	}

	var quitting atomic.Bool
	persistWindowSize := func() {
		w, h := window.Size()
		_ = settings.saveWindowSize(w, h)
	}

	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		persistWindowSize()
		if quitting.Load() {
			return
		}
		window.Hide()
		event.Cancel()
	})

	quit := func() {
		quitting.Store(true)
		persistWindowSize()
		eventService.Stop()
		app.Quit()
	}

	menu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}
	pocMenu := menu.AddSubmenu("POC")
	pocMenu.Add("Show Window").SetAccelerator("CmdOrCtrl+Shift+A").OnClick(func(*application.Context) {
		showWindow()
	})
	pocMenu.AddSeparator()
	pocMenu.Add("Quit").OnClick(func(*application.Context) {
		quit()
	})
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	app.Menu.Set(menu)

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(icons.DefaultWindowsIcon)
	}
	tray.SetTooltip("AgentDock Shared Desktop POC")
	trayMenu := app.NewMenu()
	trayMenu.Add("Show AgentDock POC").OnClick(func(*application.Context) {
		showWindow()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(*application.Context) {
		quit()
	})
	tray.SetMenu(trayMenu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func normaliseWindowSize(width, height int) (int, int) {
	if width < 760 {
		width = 1080
	}
	if height < 560 {
		height = 720
	}
	return width, height
}
