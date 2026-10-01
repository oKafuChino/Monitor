package public

import "embed"

// The share archive is built from an independent entry and never contains
// main-site assets. It travels with the existing CI frontend artifact.
//go:embed defaultTheme/share.zip
var shareFS embed.FS

func ShareArchive() ([]byte,error) { return shareFS.ReadFile("defaultTheme/share.zip") }
