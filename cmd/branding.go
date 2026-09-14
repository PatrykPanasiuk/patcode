package cmd

import (
	_ "embed"
	"fmt"
)

//go:embed ascii-art.txt
var brandArt string

var bannerOnce bool

// PrintBrandArt writes the project's ASCII banner to stdout exactly once
// per process. Running it here means the banner ships inside the binary
// (embedded), so `patcode update` carries it automatically.
func PrintBrandArt() {
	if bannerOnce {
		return
	}
	bannerOnce = true
	if brandArt == "" {
		return
	}
	fmt.Print(brandArt)
	if brandArt[len(brandArt)-1] != '\n' {
		fmt.Println()
	}
}
