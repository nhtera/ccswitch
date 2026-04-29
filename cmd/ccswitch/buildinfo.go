package main

// Build-time variables injected via -ldflags. Defaults make `go run` and
// dev builds report something coherent without any flags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)
