package winapi

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")

	procGetDiskFreeSpaceExW        = kernel32.NewProc("GetDiskFreeSpaceExW")
	procK32GetProcessMemoryInfo    = kernel32.NewProc("K32GetProcessMemoryInfo")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")

	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procEnumDisplayMonitors      = user32.NewProc("EnumDisplayMonitors")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetClassNameW            = user32.NewProc("GetClassNameW")

	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
	procPlaySoundW       = winmm.NewProc("PlaySoundW")
)

const (
	CreateNoWindow        = 0x08000000
	BelowNormalPriority   = 0x00004000
	processQueryLimited   = 0x1000
	monitorDefaultNearest = 2
)

func HideConsole(cmd *exec.Cmd, lowPriority bool) {
	flags := uint32(CreateNoWindow)
	if lowPriority {
		flags |= BelowNormalPriority
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
}

func DiskFree(path string) (free, total uint64, err error) {
	p, err := syscall.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if err != nil {
		return 0, 0, err
	}
	var avail, tot, totFree uint64
	r, _, e := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&tot)), uintptr(unsafe.Pointer(&totFree)))
	if r == 0 {
		return 0, 0, e
	}
	return avail, tot, nil
}

type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func ProcessUsage(pid int) (cpu uint64, mem uint64, err error) {
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return 0, 0, err
	}
	defer syscall.CloseHandle(h)
	var c, e, k, u syscall.Filetime
	if err := syscall.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return 0, 0, err
	}
	cpu = (uint64(k.HighDateTime)<<32 | uint64(k.LowDateTime)) + (uint64(u.HighDateTime)<<32 | uint64(u.LowDateTime))
	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	if r, _, _ := procK32GetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb)); r != 0 {
		mem = uint64(pmc.PagefileUsage)
	}
	return cpu, mem, nil
}

type Rect struct{ Left, Top, Right, Bottom int32 }

func (r Rect) W() int { return int(r.Right - r.Left) }
func (r Rect) H() int { return int(r.Bottom - r.Top) }

type monitorInfo struct {
	cbSize    uint32
	rcMonitor Rect
	rcWork    Rect
	dwFlags   uint32
}

type ForegroundInfo struct {
	PID        int
	Exe        string
	Title      string
	Class      string
	Fullscreen bool
}

func Foreground() (ForegroundInfo, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ForegroundInfo{}, errors.New("no foreground window")
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	info := ForegroundInfo{PID: int(pid)}

	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	info.Title = syscall.UTF16ToString(buf[:n])
	n, _, _ = procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	info.Class = syscall.UTF16ToString(buf[:n])

	if h, err := syscall.OpenProcess(processQueryLimited, false, pid); err == nil {
		size := uint32(len(buf))
		if r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r != 0 {
			info.Exe = syscall.UTF16ToString(buf[:size])
		}
		syscall.CloseHandle(h)
	}

	var wr Rect
	if r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr))); r != 0 {
		mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultNearest)
		mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			m := mi.rcMonitor
			info.Fullscreen = wr.Left <= m.Left && wr.Top <= m.Top && wr.Right >= m.Right && wr.Bottom >= m.Bottom
		}
	}
	return info, nil
}

type Monitor struct {
	Index   int    `json:"index"`
	Left    int    `json:"left"`
	Top     int    `json:"top"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Primary bool   `json:"primary"`
	Label   string `json:"label"`
}

var (
	monMu   sync.Mutex
	monAcc  []Monitor
	monProc = syscall.NewCallback(func(hmon, hdc, rect, data uintptr) uintptr {
		mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		if r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			monAcc = append(monAcc, Monitor{Left: int(mi.rcMonitor.Left), Top: int(mi.rcMonitor.Top), Width: mi.rcMonitor.W(), Height: mi.rcMonitor.H(), Primary: mi.dwFlags&1 != 0})
		}
		return 1
	})
)

func Monitors() []Monitor {
	monMu.Lock()
	defer monMu.Unlock()
	monAcc = nil
	procEnumDisplayMonitors.Call(0, 0, monProc, 0)
	out := monAcc
	for i := range out {
		if out[i].Primary && i != 0 {
			out[0], out[i] = out[i], out[0]
		}
	}
	for i := range out {
		out[i].Index = i
	}
	return out
}

// falls back to the main display when the saved index is from a setup with more screens
func MonitorAt(i int) Monitor {
	mons := Monitors()
	switch {
	case i >= 0 && i < len(mons):
		return mons[i]
	case len(mons) > 0:
		return mons[0]
	}
	return Monitor{Width: 1920, Height: 1080, Primary: true}
}

type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

func Recycle(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	var from []uint16
	for _, p := range paths {
		u, err := syscall.UTF16FromString(p)
		if err != nil {
			return err
		}
		from = append(from, u...)
	}
	from = append(from, 0)
	op := shFileOpStruct{
		wFunc:  3,
		pFrom:  &from[0],
		fFlags: 0x40 | 0x10 | 0x4 | 0x400,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return errors.New("recycle failed: " + strings.Join(paths, ", "))
	}
	return nil
}

var soundKeep []byte

// SND_ASYNC keeps reading the buffer after we return
func PlayWav(wav []byte) {
	soundKeep = wav
	procPlaySoundW.Call(uintptr(unsafe.Pointer(&soundKeep[0])), 0, 0x0004|0x0001|0x0002)
}
