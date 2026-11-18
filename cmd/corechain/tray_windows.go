//go:build windows

package main

import (
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
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
	procCreateIconIndir  = user32.NewProc("CreateIconIndirect")
	procDestroyIcon      = user32.NewProc("DestroyIcon")

	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateCompBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procCreatePen          = gdi32.NewProc("CreatePen")
	procRoundRect          = gdi32.NewProc("RoundRect")
	procEllipse            = gdi32.NewProc("Ellipse")
	procMoveToEx           = gdi32.NewProc("MoveToEx")
	procLineTo             = gdi32.NewProc("LineTo")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmUser       = 0x0400
	wmTrayIcon   = wmUser + 1
	wmRButtonUp  = 0x0205
	wmLButtonDbl = 0x0203
	nimAdd       = 0x00000000
	nimDelete    = 0x00000002
	nifMessage   = 0x00000001
	nifIcon      = 0x00000002
	nifTip       = 0x00000004
	mfString     = 0x00000000
	mfSeparator  = 0x00000800
	tpmReturnCmd = 0x0100

	cmdOpen      = 1001
	cmdWebChain  = 1002
	cmdLearn     = 1003
	cmdDocChain  = 1004
	cmdUpdate    = 1005
	cmdQuit      = 1006
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

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
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

func rgb(r, g, b uint32) uintptr {
	return uintptr(r | (g << 8) | (b << 16))
}

// createCoreChainTrayIcon draws a custom 32x32 CoreChain badge with interlocking chain rings.
func createCoreChainTrayIcon() uintptr {
	const size = 32
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		hIcon, _, _ := procLoadIconW.Call(0, 32512)
		return hIcon
	}
	defer procDeleteDC.Call(hdc)

	// Color bitmap (32-bit screen-compatible) and monochrome mask
	hbmColor, _, _ := procCreateCompBitmap.Call(hdc, size, size)
	hbmMask, _, _ := procCreateBitmap.Call(size, size, 1, 1, 0)
	if hbmColor == 0 || hbmMask == 0 {
		hIcon, _, _ := procLoadIconW.Call(0, 32512)
		return hIcon
	}
	defer procDeleteObject.Call(hbmColor)
	defer procDeleteObject.Call(hbmMask)

	// Fill mask with 0 (opaque)
	maskDC, _, _ := procCreateCompatibleDC.Call(0)
	oldMask, _, _ := procSelectObject.Call(maskDC, hbmMask)
	blackBrush, _, _ := procCreateSolidBrush.Call(0)
	oldMaskBrush, _, _ := procSelectObject.Call(maskDC, blackBrush)
	procRoundRect.Call(maskDC, 0, 0, size, size, 10, 10)
	procSelectObject.Call(maskDC, oldMaskBrush)
	procDeleteObject.Call(blackBrush)
	procSelectObject.Call(maskDC, oldMask)
	procDeleteDC.Call(maskDC)

	// Draw CoreChain emblem on color bitmap
	oldBmp, _, _ := procSelectObject.Call(hdc, hbmColor)
	bgBrush, _, _ := procCreateSolidBrush.Call(rgb(2, 132, 199)) // Sky-600 #0284c7
	bgPen, _, _ := procCreatePen.Call(0, 1, rgb(56, 189, 248))  // Sky-400 #38bdf8
	oldBrush, _, _ := procSelectObject.Call(hdc, bgBrush)
	oldPen, _, _ := procSelectObject.Call(hdc, bgPen)
	procRoundRect.Call(hdc, 0, 0, size, size, 10, 10)

	// Inner dark core diamond/circle
	coreBrush, _, _ := procCreateSolidBrush.Call(rgb(15, 23, 42)) // Slate-900
	procSelectObject.Call(hdc, coreBrush)
	procEllipse.Call(hdc, 5, 5, 27, 27)

	// Interlocking white & cyan chain links
	linkPen1, _, _ := procCreatePen.Call(0, 3, rgb(255, 255, 255))
	nullBrush, _, _ := procCreateSolidBrush.Call(rgb(15, 23, 42))
	procSelectObject.Call(hdc, linkPen1)
	procSelectObject.Call(hdc, nullBrush)
	procRoundRect.Call(hdc, 7, 9, 19, 19, 6, 6)

	linkPen2, _, _ := procCreatePen.Call(0, 3, rgb(56, 189, 248))
	procSelectObject.Call(hdc, linkPen2)
	procRoundRect.Call(hdc, 13, 13, 25, 23, 6, 6)

	// Center link bridge
	procMoveToEx.Call(hdc, 13, 15, 0)
	procLineTo.Call(hdc, 19, 17)

	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procSelectObject.Call(hdc, oldBmp)
	for _, obj := range []uintptr{bgBrush, bgPen, coreBrush, linkPen1, nullBrush, linkPen2} {
		procDeleteObject.Call(obj)
	}

	ii := iconInfo{
		fIcon:    1,
		hbmMask:  hbmMask,
		hbmColor: hbmColor,
	}
	hIcon, _, _ := procCreateIconIndir.Call(uintptr(unsafe.Pointer(&ii)))
	if hIcon == 0 {
		hIcon, _, _ = procLoadIconW.Call(0, 32512)
	}
	return hIcon
}

func runSystemTray(url string, onCheckUpdate func(), onQuit func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	baseURL := strings.TrimRight(url, "/")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("CoreChainTrayClass")

	wndProc := syscall.NewCallback(func(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
		if uMsg == wmTrayIcon {
			if lParam == wmLButtonDbl {
				_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL+"/").Start()
			} else if lParam == wmRButtonUp {
				menu, _, _ := procCreatePopupMenu.Call()
				addItem := func(id uintptr, text string) {
					p, _ := syscall.UTF16PtrFromString(text)
					procAppendMenuW.Call(menu, mfString, id, uintptr(unsafe.Pointer(p)))
				}
				addItem(cmdOpen, "⛓️ เปิด CoreChain Launcher")
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				addItem(cmdWebChain, "🏥 เปิด WebChain (เว็บไซต์หน่วยงาน)")
				addItem(cmdLearn, "🎓 เปิด LearnChain (E-Learning)")
				addItem(cmdDocChain, "📄 เปิด DocChain (คลังเอกสาร)")
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				addItem(cmdUpdate, "🔄 ตรวจอัปเดตพื้นหลัง (.Blz)")
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				addItem(cmdQuit, "❌ ออกจาก CoreChain")

				var pt point
				procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
				procSetForegroundWin.Call(hwnd)
				cmdID, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
				procDestroyMenu.Call(menu)

				switch cmdID {
				case cmdOpen:
					_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL+"/").Start()
				case cmdWebChain:
					_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL+"/b/webchain/").Start()
				case cmdLearn:
					_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL+"/b/learnchain/").Start()
				case cmdDocChain:
					_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL+"/b/docchain/").Start()
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

	hIcon := createCoreChainTrayIcon()
	defer procDestroyIcon.Call(hIcon)

	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayIcon
	nid.hIcon = hIcon
	tip, _ := syscall.UTF16FromString("CoreChain Decentralized Launcher (Running)")
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
