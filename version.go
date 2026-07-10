package main

// Build metadata. version defaults to the current release for `go build` /
// `go install` from source; GoReleaser overrides all three via -ldflags -X.
var (
	version = "1.1.0"
	commit  = "dev"
	date    = "unknown"
)
