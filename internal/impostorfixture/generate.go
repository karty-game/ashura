//go:build ignore

// Run from internal/impostorfixture to intentionally regenerate its golden GLB.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/karty-game/ashura/internal/impostorfixture"
)

func main() {
	data, stats, err := impostorfixture.Tree()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll("testdata", 0o750); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile("testdata/gnarled-tree.glb", data, 0o600); err != nil {
		log.Fatal(err)
	}

	fmt.Printf(
		"Authored %d triangles, %d vertices, %d material primitives; bounds %v..%v\n",
		stats.Triangles,
		stats.Vertices,
		stats.Primitives,
		stats.Min,
		stats.Max,
	)
}
