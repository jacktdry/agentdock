package main

import (
	"embed"
	"flag"
	"log"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/uvwt/agentdock/internal/desktopapi"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:frontend/dist
var assets embed.FS

const activityStreamName = "desktop:activity"

func main() {
	if err := configureProduct(); err != nil {
		log.Fatal(err)
	}
	runtimeRootFlag := flag.String("runtime-root", "", "AgentDock runtime root override")
	background := flag.Bool("background", false, "Start with the window hidden")
	flag.Parse()

	settingsPath := ""
	if *runtimeRootFlag != "" {
		settingsPath = filepath.Join(*runtimeRootFlag, "shared-desktop-poc", "preferences.json")
	}
	settings := NewSettingsService(settingsPath)
	prefs := settings.Get().Preferences
	contractService := desktopapi.NewContractService()
	runtimeService := desktopapi.NewRuntimeService(*runtimeRootFlag)
	activityProbeService := NewActivityProbeService()
	coreActivityService := NewCoreActivityService(*runtimeRootFlag)

	app := application.New(application.Options{
		Name:        productName,
		Description: "Cross-platform AgentDock desktop",
		Services: []application.Service{
			application.NewService(contractService),
			application.NewService(runtimeService),
			application.NewService(desktopapi.NewConnectionService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewBasicSettingsService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewUpdateService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewDiagnosticsService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewACPService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewMCPService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewPermissionService(*runtimeRootFlag)),
			application.NewService(settings),
			application.NewService(activityProbeService),
			application.NewService(coreActivityService),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: securityHeadersMiddleware,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	app.HandleStream(activityStreamName, activityProbeService.serveStream)
	app.HandleStream(executionStreamName, coreActivityService.serveStream)

	width, height := normaliseWindowSize(prefs.WindowWidth, prefs.WindowHeight)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:               "agentdock-shared-poc",
		Title:              productName,
		Hidden:             *background,
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
		activityProbeService.Stop()
		app.Quit()
	}

	menu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}
	desktopMenu := menu.AddSubmenu(productName)
	desktopMenu.Add("Show Window").SetAccelerator("CmdOrCtrl+Shift+A").OnClick(func(*application.Context) {
		showWindow()
	})
	desktopMenu.AddSeparator()
	desktopMenu.Add("Quit").OnClick(func(*application.Context) {
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
	tray.SetTooltip(productName)
	trayMenu := app.NewMenu()
	trayMenu.Add("Show " + productName).OnClick(func(*application.Context) {
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
