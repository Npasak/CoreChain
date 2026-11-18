//go:build !windows

package main

func runSystemTray(url string, onCheckUpdate func(), onQuit func()) {
	// Headless on non-Windows
}
