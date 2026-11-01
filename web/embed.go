// Package web embeds the built admin UI (web/dist). Without a UI build only .gitkeep is embedded.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
