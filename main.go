/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"embed"

	"github.com/openshift/sippy/cmd"
)

//go:embed sippy-ng/build
var sippyNG embed.FS

//go:embed static
var static embed.FS

func main() {
	cmd.Execute(sippyNG, static)
}
