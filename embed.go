package resources

import "embed"

//go:embed sippy-ng/build
var SippyNG embed.FS

//go:embed static
var Static embed.FS
