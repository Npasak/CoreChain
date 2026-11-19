//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// openAppWindow launches the URL in a standalone chromeless desktop app window
// (no address bar, no tabs, no browser menus) using Microsoft Edge or Chrome --app mode.
func openAppWindow(url string) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}

	profileDir := filepath.Join(os.Getenv("LocalAppData"), "CoreChain", "AppProfile")
	_ = os.MkdirAll(profileDir, 0755)

	for _, bin := range candidates {
		if bin == "" {
			continue
		}
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			cmd := exec.Command(
				bin,
				"--app="+url,
				"--user-data-dir="+profileDir,
				"--window-size=1280,840",
				"--disable-features=msEdgeSidebarV2",
			)
			if err := cmd.Start(); err == nil {
				return
			}
		}
	}

	// Fallback to default browser if Edge/Chrome is not found
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
