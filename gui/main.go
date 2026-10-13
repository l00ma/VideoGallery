// The Video Gallery : interface graphique (Wails) au-dessus du moteur de
// génération. Le frontend est du HTML/CSS/JS sans étape de build.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend
var assets embed.FS

// Icône de fenêtre sous Linux (sous Windows, wails build l'intègre à l'exe).
//
//go:embed build/appicon.png
var icon []byte

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "The Video Gallery",
		Width:     850,
		Height:    700,
		MinWidth:  640,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
		Linux:     &linux.Options{Icon: icon},
	})
	if err != nil {
		log.Fatal(err)
	}
}
