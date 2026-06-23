package templates

import "embed"

//go:embed project
var Project embed.FS

//go:embed all:source
var Source embed.FS
