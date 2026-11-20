//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	procLoadImageW       = user32.NewProc("LoadImageW")
	procCreateIconIndir  = user32.NewProc("CreateIconIndirect")
	procDestroyIcon               = user32.NewProc("DestroyIcon")
	procMessageBoxW               = user32.NewProc("MessageBoxW")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem           = user32.NewProc("GetDpiForSystem")
	procSetMenuItemInfoW          = user32.NewProc("SetMenuItemInfoW")

	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateCompBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
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
	cmdRestart   = 1006
	cmdQuit      = 1007
	cmdShortcut  = 1008
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

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type menuItemInfoW struct {
	cbSize        uint32
	fMask         uint32
	fType         uint32
	fState        uint32
	wID           uint32
	hSubMenu      uintptr
	hbmpChecked   uintptr
	hbmpUnchecked uintptr
	dwItemData    uintptr
	dwTypeData    *uint16
	cch           uint32
	hbmpItem      uintptr
}

const miimBitmap = 0x00000080

func rgb(r, g, b uint32) uintptr {
	return uintptr(r | (g << 8) | (b << 16))
}

// createColorMenuBitmap creates a crisp 32-bit ARGB colored badge icon for Win32 popup menus.
func createColorMenuBitmap(size int, r, g, b uint8, kind string) uintptr {
	if size < 16 {
		size = 16
	}
	var bi bitmapInfoHeader
	bi.biSize = uint32(unsafe.Sizeof(bi))
	bi.biWidth = int32(size)
	bi.biHeight = -int32(size) // top-down DIB
	bi.biPlanes = 1
	bi.biBitCount = 32
	bi.biCompression = 0 // BI_RGB

	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		return 0
	}

	pixels := unsafe.Slice((*uint32)(bits), size*size)
	bg := (uint32(0xFF) << 24) | (uint32(r) << 16) | (uint32(g) << 8) | uint32(b)
	fg := uint32(0xFFFFFFFF)

	rad := float64(size) * 0.24
	fSize := float64(size)
	setPx := func(x, y int, col uint32) {
		if x >= 0 && x < size && y >= 0 && y < size {
			pixels[y*size+x] = col
		}
	}
	inRoundedRect := func(x, y int) bool {
		fx, fy := float64(x)+0.5, float64(y)+0.5
		if fx < rad && fy < rad {
			dx, dy := rad-fx, rad-fy
			return dx*dx+dy*dy <= rad*rad
		}
		if fx > fSize-rad && fy < rad {
			dx, dy := fx-(fSize-rad), rad-fy
			return dx*dx+dy*dy <= rad*rad
		}
		if fx < rad && fy > fSize-rad {
			dx, dy := rad-fx, fy-(fSize-rad)
			return dx*dx+dy*dy <= rad*rad
		}
		if fx > fSize-rad && fy > fSize-rad {
			dx, dy := fx-(fSize-rad), fy-(fSize-rad)
			return dx*dx+dy*dy <= rad*rad
		}
		return true
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if inRoundedRect(x, y) {
				pixels[y*size+x] = bg
			}
		}
	}

	// Draw crisp white symbol inside the colored badge (normalized 0..1 coordinates)
	fillNorm := func(x0, y0, x1, y1 float64) {
		ix0, iy0 := int(x0*fSize), int(y0*fSize)
		ix1, iy1 := int(x1*fSize), int(y1*fSize)
		for y := iy0; y <= iy1; y++ {
			for x := ix0; x <= ix1; x++ {
				setPx(x, y, fg)
			}
		}
	}

	switch kind {
	case "core":
		// Interlocking diamond / chain mark
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				nx := (float64(x)+0.5)/fSize - 0.5
				ny := (float64(y)+0.5)/fSize - 0.5
				if nx < 0 {
					nx = -nx
				}
				if ny < 0 {
					ny = -ny
				}
				d := nx + ny
				if d >= 0.14 && d <= 0.30 {
					setPx(x, y, fg)
				}
			}
		}
	case "web":
		// Medical cross (+)
		fillNorm(0.42, 0.22, 0.58, 0.78)
		fillNorm(0.22, 0.42, 0.78, 0.58)
	case "learn":
		// Graduation cap / open book
		fillNorm(0.22, 0.34, 0.78, 0.48)
		fillNorm(0.32, 0.50, 0.68, 0.68)
	case "doc":
		// Document lines
		fillNorm(0.28, 0.26, 0.72, 0.36)
		fillNorm(0.28, 0.45, 0.72, 0.55)
		fillNorm(0.28, 0.64, 0.58, 0.74)
	case "shortcut":
		// Pin / star square
		fillNorm(0.36, 0.24, 0.64, 0.52)
		fillNorm(0.45, 0.52, 0.55, 0.78)
	case "update":
		// Up arrow
		fillNorm(0.43, 0.36, 0.57, 0.76)
		for y := int(0.20 * fSize); y <= int(0.46*fSize); y++ {
			ny := float64(y)/fSize - 0.20
			half := ny * 1.1
			for x := int((0.5 - half) * fSize); x <= int((0.5 + half) * fSize); x++ {
				setPx(x, y, fg)
			}
		}
	case "restart":
		// Circular ring with gap
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				nx := (float64(x)+0.5)/fSize - 0.5
				ny := (float64(y)+0.5)/fSize - 0.5
				r2 := nx*nx + ny*ny
				if r2 >= 0.045 && r2 <= 0.11 && !(nx > 0.05 && ny < -0.05) {
					setPx(x, y, fg)
				}
			}
		}
	case "exit":
		// Crisp X
		for y := int(0.26 * fSize); y <= int(0.74*fSize); y++ {
			for x := int(0.26 * fSize); x <= int(0.74*fSize); x++ {
				nx := float64(x) / fSize
				ny := float64(y) / fSize
				d1 := nx - ny
				d2 := nx + ny - 1.0
				if d1 < 0 {
					d1 = -d1
				}
				if d2 < 0 {
					d2 = -d2
				}
				if d1 < 0.09 || d2 < 0.09 {
					setPx(x, y, fg)
				}
			}
		}
	}
	return hbmp
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

func createDesktopShortcut() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	workDir := filepath.Dir(exePath)
	escExe := strings.ReplaceAll(exePath, "'", "''")
	escDir := strings.ReplaceAll(workDir, "'", "''")
	psScript := fmt.Sprintf(
		`$s=(New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path ([Environment]::GetFolderPath('Desktop')) 'CoreChain HUB.lnk'));`+
			`$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.IconLocation='%s,0';$s.Description='CoreChain HUB — One Core. Infinite Blocks.';$s.Save()`,
		escExe, escDir, escExe,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	err = cmd.Run()
	go func() {
		title, _ := syscall.UTF16PtrFromString("CoreChain HUB")
		msgText := "สร้าง Shortcut 'CoreChain HUB' บน Desktop เรียบร้อยแล้ว"
		if err != nil {
			msgText = "ไม่สามารถสร้าง Shortcut ได้: " + err.Error()
		}
		text, _ := syscall.UTF16PtrFromString(msgText)
		procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x00000040)
	}()
}

func runSystemTray(url string, onCheckUpdate func(), onRestart func(), onQuit func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Enable Per-Monitor DPI Awareness v2 so tray menu font & icons match native Windows 11 apps
	_, _, _ = procSetProcessDpiAwarenessCtx.Call(^uintptr(3))

	baseURL := strings.TrimRight(url, "/")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("CoreChainTrayClass")

	wndProc := syscall.NewCallback(func(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
		if uMsg == wmTrayIcon {
			if lParam == wmLButtonDbl {
				openAppWindow(baseURL + "/")
			} else if lParam == wmRButtonUp {
				dpi, _, _ := procGetDpiForSystem.Call()
				if dpi == 0 {
					dpi = 96
				}
				iconSize := int(16 * dpi / 96)
				if iconSize < 16 {
					iconSize = 16
				}

				menu, _, _ := procCreatePopupMenu.Call()
				var bitmaps []uintptr
				addItem := func(id uintptr, text string, r, g, b uint8, kind string) {
					p, _ := syscall.UTF16PtrFromString(text)
					procAppendMenuW.Call(menu, mfString, id, uintptr(unsafe.Pointer(p)))
					hbmp := createColorMenuBitmap(iconSize, r, g, b, kind)
					if hbmp != 0 {
						bitmaps = append(bitmaps, hbmp)
						var mii menuItemInfoW
						mii.cbSize = uint32(unsafe.Sizeof(mii))
						mii.fMask = miimBitmap
						mii.hbmpItem = hbmp
						procSetMenuItemInfoW.Call(menu, id, 0, uintptr(unsafe.Pointer(&mii)))
					}
				}
				addItem(cmdOpen, "CoreChain HUB", 79, 70, 229, "core")
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				addItem(cmdWebChain, "WebChain", 16, 185, 129, "web")
				addItem(cmdLearn, "LearnChain", 2, 132, 199, "learn")
				addItem(cmdDocChain, "DocChain", 217, 119, 6, "doc")
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				addItem(cmdShortcut, "Shortcut", 124, 58, 237, "shortcut")
				addItem(cmdUpdate, "Update", 8, 145, 178, "update")
				addItem(cmdRestart, "Restart", 234, 88, 12, "restart")
				procAppendMenuW.Call(menu, mfSeparator, 0, 0)
				addItem(cmdQuit, "Exit", 220, 38, 38, "exit")

				var pt point
				procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
				procSetForegroundWin.Call(hwnd)
				cmdID, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
				procDestroyMenu.Call(menu)
				for _, hbmp := range bitmaps {
					procDeleteObject.Call(hbmp)
				}

				switch cmdID {
				case cmdOpen:
					openAppWindow(baseURL + "/")
				case cmdWebChain:
					openAppWindow(baseURL + "/b/webchain/")
				case cmdLearn:
					openAppWindow(baseURL + "/b/learnchain/")
				case cmdDocChain:
					openAppWindow(baseURL + "/b/docchain/")
				case cmdShortcut:
					go createDesktopShortcut()
				case cmdUpdate:
					if onCheckUpdate != nil {
						onCheckUpdate()
					}
				case cmdRestart:
					if onRestart != nil {
						onRestart()
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

	hIcon, _, _ := procLoadImageW.Call(hInst, 1, 1, 32, 32, 0) // IMAGE_ICON=1, resource ID=1 from corechain.ico/.syso
	if hIcon == 0 {
		hIcon = createCoreChainTrayIcon()
	}
	defer procDestroyIcon.Call(hIcon)

	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayIcon
	nid.hIcon = hIcon
	tip, _ := syscall.UTF16FromString("CoreChain HUB — One Core. Infinite Blocks.")
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
