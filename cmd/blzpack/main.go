package main

import (
	"fmt"
	"log"
	"os"

	"corechain/sdk/blocksdk"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: blzpack <block-dir> <output.Blz>")
		os.Exit(1)
	}
	srcDir := os.Args[1]
	outPath := os.Args[2]

	manifest, err := blocksdk.ReadManifest(srcDir)
	if err != nil {
		log.Fatalf("Invalid block directory %s: %v", srcDir, err)
	}
	hash, err := blocksdk.PackBlz(srcDir, outPath)
	if err != nil {
		log.Fatalf("Failed to pack .Blz: %v", err)
	}
	fmt.Printf("Packed %s v%s -> %s (sha256:%s)\n", manifest.ID, manifest.Version, outPath, hash[:16])
}
