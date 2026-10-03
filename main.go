package main

import (
	"embed"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/qnqatop/papeer/internal/app"
	"github.com/qnqatop/papeer/internal/server"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := app.NewApp()
	pdfHandler := app.NewPDFHandler(a)

	err := wails.Run(&options.App{
		Title:  "Papeer",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Middleware rather than Handler: in `wails dev` the Vite server
			// answers unknown paths with index.html (SPA fallback), so a
			// fall-through Handler would never see /api/pdf requests.
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasPrefix(r.URL.Path, server.PDFPathPrefix) {
						pdfHandler.ServeHTTP(w, r)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        a.Startup,
		OnShutdown:       a.Shutdown,
		Bind: []interface{}{
			a,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
