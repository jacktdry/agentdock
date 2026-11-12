//go:build !darwin

// The standalone observer proof is macOS-only; non-Darwin always refuses.
package main

import "os"

func main() { os.Exit(1) }
