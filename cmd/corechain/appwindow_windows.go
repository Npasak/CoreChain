//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

var onFreshWindowOpen func()

func isAppWindowRunning() bool {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-WindowStyle", "Hidden",
		"-Command",
		`if (Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -like '*CoreChain\AppProfile*' } | Select-Object -First 1) { exit 0 } else { exit 1 }`,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	return cmd.Run() == nil
}

// openAppWindow launches the URL in a standalone chromeless desktop app window
// (no address bar, no tabs, no browser menus) using Microsoft Edge or Chrome --app mode.
func openAppWindow(url string) {
	if onFreshWindowOpen != nil && !isAppWindowRunning() {
		onFreshWindowOpen()
	}

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

// closeAppWindows closes any standalone CoreChain Edge/Chrome app windows without affecting the user's normal browser.
func closeAppWindows() {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-WindowStyle", "Hidden",
		"-Command",
		`Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -like '*CoreChain\AppProfile*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	_ = cmd.Run()
}
