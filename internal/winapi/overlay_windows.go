package winapi

import (
	_ "embed"
	"math"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed fonts/toast-semibold.ttf
var fontSemibold []byte

//go:embed fonts/toast-regular.ttf
var fontRegular []byte

var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetTimer                 = user32.NewProc("SetTimer")
	procKillTimer                = user32.NewProc("KillTimer")
	procDrawTextW                = user32.NewProc("DrawTextW")
	procUpdateLayeredWindow      = user32.NewProc("UpdateLayeredWindow")
	procSetWindowDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")
	procGetDpiForSystem          = user32.NewProc("GetDpiForSystem")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")

	procCreateFontW          = gdi32.NewProc("CreateFontW")
	procSelectObject         = gdi32.NewProc("SelectObject")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procDeleteDC             = gdi32.NewProc("DeleteDC")
	procSetTextColor         = gdi32.NewProc("SetTextColor")
	procSetBkMode            = gdi32.NewProc("SetBkMode")
	procCreateCompatibleDC   = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection     = gdi32.NewProc("CreateDIBSection")
	procAddFontMemResourceEx = gdi32.NewProc("AddFontMemResourceEx")
	procGdiFlush             = gdi32.NewProc("GdiFlush")
)

const (
	wmTimer     = 0x0113
	wmNCHitTest = 0x0084
	wmApp       = 0x8000

	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExTransparent = 0x00000020
	wsExLayered     = 0x00080000
	wsExNoActivate  = 0x08000000

	swHide            = 0
	swShowNoActivate  = 4
	htTransparent     = ^uintptr(0)
	wdaExcludeCapture = 0x11
	ulwAlpha          = 0x2
	dtSingleLine      = 0x20
	dtVCenter         = 0x4
	dtEndEllipsis     = 0x8000
	dtNoPrefix        = 0x800

	timerAnim = 1
	timerHold = 2
)

type wndClassEx struct {
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

type msg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX, ptY int32
	lPrivate uint32
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type point struct{ x, y int32 }
type size struct{ cx, cy int32 }

type toast struct {
	view   toastView
	sticky bool
	corner string
	mon    Monitor
}

type phase int

const (
	hidden phase = iota
	entering
	holding
	leaving
)

var (
	ovOnce  sync.Once
	ovHwnd  uintptr
	ovMu    sync.Mutex
	ovState toast
	ovScale = 1.0
	ovProc  = syscall.NewCallback(overlayProc)

	// overlay thread only
	ovPhase   phase
	ovStart   time.Time
	ovFrame   []byte
	ovW, ovH  int
	ovX, ovY  int32
	ovFromTop bool
)

const (
	enterDur = 170 * time.Millisecond
	leaveDur = 200 * time.Millisecond
	holdDur  = 2600 * time.Millisecond
)

func easeOut(t float64) float64 { return 1 - math.Pow(1-math.Min(1, math.Max(0, t)), 3) }

func newDIB(dc uintptr, w, h int) (uintptr, unsafe.Pointer) {
	bi := bitmapInfoHeader{width: int32(w), height: -int32(h), planes: 1, bitCount: 32}
	bi.size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	return bmp, bits
}

// let gdi hint the text, then use it as a mask
func gdiText(s string, semibold bool, sizePx, w, h int) []uint8 {
	if s == "" || w <= 0 || h <= 0 {
		return nil
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(dc)
	bmp, bits := newDIB(dc, w, h)
	if bmp == 0 || bits == nil {
		return nil
	}
	defer procDeleteObject.Call(bmp)
	oldBmp, _, _ := procSelectObject.Call(dc, bmp)
	defer procSelectObject.Call(dc, oldBmp)

	face := "Snag Toast Regular"
	if semibold {
		face = "Snag Toast Semibold"
	}
	fp, _ := syscall.UTF16PtrFromString(face)
	// not cleartype, colour fringes break the mask
	font, _, _ := procCreateFontW.Call(uintptr(-int32(sizePx)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(fp)))
	defer procDeleteObject.Call(font)
	oldFont, _, _ := procSelectObject.Call(dc, font)
	defer procSelectObject.Call(dc, oldFont)
	procSetBkMode.Call(dc, 1)
	procSetTextColor.Call(dc, 0xffffff)
	t, _ := syscall.UTF16FromString(s)
	r := Rect{0, 0, int32(w), int32(h)}
	procDrawTextW.Call(dc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), dtSingleLine|dtVCenter|dtEndEllipsis|dtNoPrefix)
	procGdiFlush.Call()

	src := unsafe.Slice((*byte)(bits), w*h*4)
	out := make([]uint8, w*h)
	for i := range out {
		out[i] = src[i*4+1]
	}
	return out
}

func layout(st toast, w, h int) (x, y int32, fromTop bool) {
	pad := int32(px2(tPad, ovScale))
	margin := int32(px2(20, ovScale))
	x = int32(st.mon.Left) + margin - pad
	y = int32(st.mon.Top) + margin - pad
	fromTop = true
	if st.corner == "top-right" || st.corner == "bottom-right" {
		x = int32(st.mon.Left+st.mon.Width) - int32(w) - margin + pad
	}
	if st.corner == "bottom-left" || st.corner == "bottom-right" {
		y = int32(st.mon.Top+st.mon.Height) - int32(h) - margin + pad - int32(px2(44, ovScale))
		fromTop = false
	}
	return
}

func present(hwnd uintptr, alpha float64, offset int32) {
	if ovFrame == nil {
		return
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(dc)
	bmp, bits := newDIB(dc, ovW, ovH)
	if bmp == 0 || bits == nil {
		return
	}
	defer procDeleteObject.Call(bmp)
	copy(unsafe.Slice((*byte)(bits), len(ovFrame)), ovFrame)
	old, _, _ := procSelectObject.Call(dc, bmp)
	defer procSelectObject.Call(dc, old)

	dst := point{ovX, ovY + offset}
	sz := size{int32(ovW), int32(ovH)}
	src := point{}
	a := uint32(math.Round(math.Min(1, math.Max(0, alpha)) * 255))
	// BLENDFUNCTION{AC_SRC_OVER, 0, alpha, AC_SRC_ALPHA}
	blend := a<<16 | 1<<24
	procUpdateLayeredWindow.Call(hwnd, screen, uintptr(unsafe.Pointer(&dst)), uintptr(unsafe.Pointer(&sz)), dc,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

func slide() int32 {
	d := int32(px2(10, ovScale))
	if ovFromTop {
		return -d
	}
	return d
}

func holdIfDone(hwnd uintptr) {
	ovMu.Lock()
	sticky := ovState.sticky
	ovMu.Unlock()
	if !sticky {
		procSetTimer.Call(hwnd, timerHold, uintptr(holdDur/time.Millisecond), 0)
	}
}

func overlayProc(hwnd, m, wp, lp uintptr) uintptr {
	switch m {
	case wmNCHitTest:
		return htTransparent
	case wmApp:
		ovMu.Lock()
		st := ovState
		ovMu.Unlock()
		ovW, ovH, ovFrame = renderToast(st.view, ovScale, gdiText)
		ovX, ovY, ovFromTop = layout(st, ovW, ovH)
		procKillTimer.Call(hwnd, timerHold)
		switch ovPhase {
		case hidden, leaving:
			ovPhase = entering
			ovStart = time.Now()
			present(hwnd, 0, slide())
			procShowWindow.Call(hwnd, swShowNoActivate)
			procSetTimer.Call(hwnd, timerAnim, 15, 0)
		case entering:
		default:
			present(hwnd, 1, 0)
			holdIfDone(hwnd)
		}
		return 0
	case wmTimer:
		switch wp {
		case timerHold:
			procKillTimer.Call(hwnd, timerHold)
			ovPhase = leaving
			ovStart = time.Now()
			procSetTimer.Call(hwnd, timerAnim, 15, 0)
		case timerAnim:
			t := float64(time.Since(ovStart))
			switch ovPhase {
			case entering:
				k := easeOut(t / float64(enterDur))
				present(hwnd, k, int32(float64(slide())*(1-k)))
				if k >= 1 {
					procKillTimer.Call(hwnd, timerAnim)
					ovPhase = holding
					holdIfDone(hwnd)
				}
			case leaving:
				k := easeOut(t / float64(leaveDur))
				present(hwnd, 1-k, int32(float64(slide())*k*0.5))
				if k >= 1 {
					procKillTimer.Call(hwnd, timerAnim)
					procShowWindow.Call(hwnd, swHide)
					ovPhase = hidden
				}
			default:
				procKillTimer.Call(hwnd, timerAnim)
			}
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// win32 windows get messages on the thread that created them
func startOverlay() {
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi > 0 {
			ovScale = float64(dpi) / 96
		}
		// process-private, nothing is installed
		for _, f := range [][]byte{fontSemibold, fontRegular} {
			var n uint32
			procAddFontMemResourceEx.Call(uintptr(unsafe.Pointer(&f[0])), uintptr(len(f)), 0, uintptr(unsafe.Pointer(&n)))
		}
		inst, _, _ := procGetModuleHandleW.Call(0)
		name, _ := syscall.UTF16PtrFromString("SnagToast")
		wc := wndClassEx{lpfnWndProc: ovProc, hInstance: inst, lpszClassName: name}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		ovHwnd, _, _ = procCreateWindowExW.Call(
			wsExTopmost|wsExToolWindow|wsExTransparent|wsExLayered|wsExNoActivate,
			uintptr(unsafe.Pointer(name)), 0, wsPopup, 0, 0, 10, 10, 0, 0, inst, 0)
		if ovHwnd != 0 {
			// otherwise the popup ends up in clips
			procSetWindowDisplayAffinity.Call(ovHwnd, wdaExcludeCapture)
		}
		close(ready)
		var m msg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	<-ready
}

func post(t toast) {
	ovOnce.Do(startOverlay)
	if ovHwnd == 0 {
		return
	}
	ovMu.Lock()
	ovState = t
	ovMu.Unlock()
	procPostMessageW.Call(ovHwnd, wmApp, 0, 0)
}

func ShowToast(kind ToastKind, title, sub, corner string, mon Monitor) {
	post(toast{view: toastView{kind: kind, title: title, sub: sub, progress: -1}, corner: corner, mon: mon})
}

func ShowToastProgress(title, sub string, progress float64, corner string, mon Monitor) {
	post(toast{view: toastView{kind: ToastInfo, title: title, sub: sub, progress: progress}, sticky: true, corner: corner, mon: mon})
}
