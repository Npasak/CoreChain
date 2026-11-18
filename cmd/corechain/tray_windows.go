//go:build windows

package main

import (
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	shell32              = syscall.NewLazyDLL("shell32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmUser        = 0x0400
	wmTrayIcon    = wmUser + 1
	wmRButtonUp   = 0x0205
	wmLButtonDbl  = 0x0203
	wmCommand     = 0x0111
	nimAdd        = 0x00000000
	nimDelete     = 0x00000002
	nifMessage    = 0x00000001
	nifIcon       = 0x00000002
	nifTip        = 0x00000004
	mfString      = 0x00000000
	mfSeparator   = 0x00000800
	tpmReturnCmd  = 0x0100
	cmdOpen       = 1001
	cmdUpdate     = 1002
	cmdQuit       = 1003
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
}

type point struct {
	x, y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

func runSystemTray(url string, onCheckUpdate func(), onQuit func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("CoreChainTrayClass")

	wndProc := syscall.NewCallback(func(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
		if uMsg == wmTrayIcon {
			if lParam == wmLButtonDbl {
				_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
			} else if lParam == wmRButtonUp {
				menu, _, _ := procCreatePopupMenu.Call()
				s1, _ := syscall.UTF16PtrFromString("Open CoreChain Launcher")
				s2, _ := syscall.UTF16PtrFromString("Check Background Updates")
				s3, _ := syscall.UTF16PtrFromString("Quit CoreChain")
				procAppendMenuW.Call(menu, mfString, cmdOpen, uintptr(unsafe.Pointer(s1)))
				procAppendMenuW.Call(menu, mfString, cmdUpdate, uintptr(unsafe.Pointer(s2)))
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				procAppendMenuW.Call(menu, mfString, cmdQuit, uintptr(unsafe.Pointer(s3)))

				var pt point
				procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
				procSetForegroundWin.Call(hwnd)
				cmdID, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
				procDestroyMenu.Call(menu)

				switch cmdID {
				case cmdOpen:
					_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
				case cmdUpdate:
					if onCheckUpdate != nil {
						onCheckUpdate()
					}
				case cmdQuit:
					if onQuit != nil {
						onQuit()
					}
				}
			}
			return 0
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(uMsg), wParam, lParam)
		return ret
	})

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProc,
		hInstance:     hInst,
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := syscall.UTF16PtrFromString("CoreChain")
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}

	hIcon, _, _ := procLoadIconW.Call(0, 32512) // IDI_APPLICATION
	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayIcon
	nid.hIcon = hIcon
	tip, _ := syscall.UTF16FromString("CoreChain Decentralized Launcher")
	copy(nid.szTip[:], tip)
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	defer procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
