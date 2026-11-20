//go:build !windows

package main

func runSystemTray(url string, onCheckUpdate func(), onRestart func(), onQuit func()) {
	// Headless on non-Windows
}
