package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const aboutWindow = "about"

// The page opens links by name: it never hands an address to the browser itself.
var links = map[string]string{
	"sponsors": "https://github.com/sponsors/vmaerten",
	"coffee":   "https://buymeacoffee.com/vmaerten",
	"website":  "https://vmaerten.fr",
	"github":   "https://github.com/vmaerten",
	"code":     "https://github.com/vmaerten/epuisette",
	"issues":   "https://github.com/vmaerten/epuisette/issues/new",
	"changes":  "https://github.com/vmaerten/epuisette/releases",
}

func openLink(name string) error {
	url, ok := links[name]
	if !ok {
		return fmt.Errorf("unknown link %q", name)
	}
	return application.Get().Browser.OpenURL(url)
}

// showAbout brings the About window forward, opening it the first time.
func showAbout() {
	app := application.Get()
	if window, ok := app.Window.GetByName(aboutWindow); ok {
		window.Show()
		window.Focus()
		return
	}
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                aboutWindow,
		Title:               "À propos d'Épuisette",
		URL:                 "/about.html",
		Width:               380,
		Height:              540,
		DisableResize:       true,
		MinimiseButtonState: application.ButtonDisabled,
		MaximiseButtonState: application.ButtonDisabled,
	})
}

// applicationMenu is the default macOS menu, with About and Support entries of our own.
func applicationMenu() *application.Menu {
	menu := application.NewMenu()
	appMenu := menu.AddSubmenu("Épuisette")
	appMenu.Add("À propos d'Épuisette").OnClick(func(*application.Context) { showAbout() })
	appMenu.Add("Soutenir Épuisette…").OnClick(func(*application.Context) { _ = openLink("sponsors") })
	appMenu.AddSeparator()
	appMenu.AddRole(application.ServicesMenu)
	appMenu.AddSeparator()
	appMenu.AddRole(application.Hide)
	appMenu.AddRole(application.HideOthers)
	// The UnHide role has no selector on macOS; ShowAll does.
	appMenu.AddRole(application.ShowAll)
	appMenu.AddSeparator()
	appMenu.AddRole(application.Quit)
	// Without the Edit menu, Cmd+C and Cmd+V stop working in the page.
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	return menu
}
