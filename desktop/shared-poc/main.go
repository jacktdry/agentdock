package main

import (
	"context"
	"embed"
	"errors"
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
	nativeLabels := labelsForNativeMenu(nativeMenuLanguage())
	settings := NewSettingsService(settingsPath)
	prefs := settings.Get().Preferences
	contractService := desktopapi.NewContractService()
	runtimeService := desktopapi.NewRuntimeService(*runtimeRootFlag)
	activityProbeService := NewActivityProbeService()
	coreActivityService := NewCoreActivityService(*runtimeRootFlag)
	var app *application.App
	mcpService := desktopapi.NewMCPServiceWithOpenURL(*runtimeRootFlag, func(rawURL string) error {
		if app == nil || app.Browser == nil {
			return errors.New("desktop browser unavailable")
		}
		return app.Browser.OpenURL(rawURL)
	})
	pluginService := desktopapi.NewPluginServiceWithPicker(*runtimeRootFlag, func(sourceType string) (string, error) {
		if app == nil || app.Dialog == nil {
			return "", errors.New("desktop file picker unavailable")
		}
		switch sourceType {
		case "folder":
			return app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
				CanChooseDirectories: true,
				CanChooseFiles:       false,
				CanCreateDirectories: false,
				ResolvesAliases:      true,
				Title:                nativeLabels.pluginFolder,
				ButtonText:           nativeLabels.choose,
			}).PromptForSingleSelection()
		case "zip":
			return app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
				CanChooseDirectories: false,
				CanChooseFiles:       true,
				CanCreateDirectories: false,
				ResolvesAliases:      true,
				AllowsOtherFileTypes: false,
				Filters: []application.FileFilter{
					{DisplayName: nativeLabels.zipArchives, Pattern: "*.zip"},
				},
				Title:      nativeLabels.pluginZip,
				ButtonText: nativeLabels.choose,
			}).PromptForSingleSelection()
		default:
			return "", errors.New("unsupported Plugin source type")
		}
	})

	app = application.New(application.Options{
		Name:        productName,
		Description: "Cross-platform AgentDock desktop",
		Services: []application.Service{
			application.NewService(contractService),
			application.NewService(runtimeService),
			application.NewService(desktopapi.NewConnectionService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewNexusService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewBasicSettingsService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewUpdateService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewDiagnosticsService(*runtimeRootFlag)),
			application.NewService(desktopapi.NewACPService(*runtimeRootFlag)),
			application.NewService(mcpService),
			application.NewService(desktopapi.NewBrowserService(*runtimeRootFlag)),
			application.NewService(pluginService),
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

	coreProbeClose := func() {}
	quit := func() {
		quitting.Store(true)
		persistWindowSize()
		activityProbeService.Stop()
		coreProbeClose()
		app.Quit()
	}

	menu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}
	desktopMenu := menu.AddSubmenu(productName)
	desktopMenu.Add(nativeLabels.showWindow).SetAccelerator("CmdOrCtrl+Shift+A").OnClick(func(*application.Context) {
		showWindow()
	})
	desktopMenu.AddSeparator()
	desktopMenu.Add(nativeLabels.quit).OnClick(func(*application.Context) {
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
	coreStatusItem := trayMenu.Add(nativeLabels.trayInitial).SetEnabled(false)
	coreProbe := newCoreTrayProbe(nativeLabels,
		func(ctx context.Context) coreTrayReading {
			result := runtimeService.Status(ctx)
			return coreTrayReading{running: result.Status.Running, healthy: result.Status.Healthy, err: result.Error != nil}
		},
		application.InvokeAsync,
		func(label string) { coreStatusItem.SetLabel(label) },
	)
	coreProbeClose = coreProbe.Close
	trayMenu.Add(nativeLabels.trayRefresh).OnClick(func(*application.Context) {
		coreProbe.Refresh()
	})
	trayMenu.AddSeparator()
	trayMenu.Add(nativeLabels.open + " " + productName).OnClick(func(*application.Context) {
		showWindow()
	})
	trayMenu.AddSeparator()
	trayMenu.Add(nativeLabels.quit).OnClick(func(*application.Context) {
		quit()
	})
	tray.SetMenu(trayMenu)

	err := app.Run()
	coreProbeClose()
	if err != nil {
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
