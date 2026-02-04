// Package ui embeds the frontend static files into the binary.
package ui

import "embed"

//go:embed all:dist
var Assets embed.FS
