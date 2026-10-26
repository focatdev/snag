package winapi

import (
	"errors"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	procOpenClipboard                    = user32.NewProc("OpenClipboard")
	procCloseClipboard                   = user32.NewProc("CloseClipboard")
	procEmptyClipboard                   = user32.NewProc("EmptyClipboard")
	procSetClipboardData                 = user32.NewProc("SetClipboardData")
	procRegisterClipboardFormatW         = user32.NewProc("RegisterClipboardFormatW")
	procCreateWindowExW                  = user32.NewProc("CreateWindowExW")
	procDestroyWindow                    = user32.NewProc("DestroyWindow")
	procGlobalAlloc                      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock                     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree                       = kernel32.NewProc("GlobalFree")
	hwndMessage                  uintptr = ^uintptr(2)
)

const (
	cfHDrop       = 15
	cfUnicodeText = 13
	gmemMoveable  = 0x2
	dropFilesSize = 20
)

func globalBytes(data []byte) (uintptr, error) {
	h, _, e := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return 0, e
	}
	p, _, e := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return 0, e
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	procGlobalUnlock.Call(h)
	return h, nil
}

func withClipboard(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// SetClipboardData needs an owner window
	cls, _ := syscall.UTF16PtrFromString("STATIC")
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, 0, 0)
	if hwnd != 0 {
		defer procDestroyWindow.Call(hwnd)
	}

	opened := false
	for i := 0; i < 20; i++ {
		if r, _, _ := procOpenClipboard.Call(hwnd); r != 0 {
			opened = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !opened {
		return errors.New("clipboard is busy")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	return fn()
}

func CopyText(text string) error {
	return withClipboard(func() error {
		u, err := syscall.UTF16FromString(text)
		if err != nil {
			return err
		}
		data := make([]byte, 0, len(u)*2)
		for _, c := range u {
			data = append(data, byte(c), byte(c>>8))
		}
		h, err := globalBytes(data)
		if err != nil {
			return err
		}
		if r, _, e := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
			procGlobalFree.Call(h)
			return e
		}
		return nil
	})
}

// CF_HDROP so ctrl+v in discord attaches the file
func CopyFiles(paths ...string) error {
	if len(paths) == 0 {
		return errors.New("nothing to copy")
	}
	return withClipboard(func() error {
		data := make([]byte, dropFilesSize)
		data[0] = dropFilesSize
		data[16] = 1
		for _, p := range paths {
			u, err := syscall.UTF16FromString(p)
			if err != nil {
				return err
			}
			for _, c := range u {
				data = append(data, byte(c), byte(c>>8))
			}
		}
		data = append(data, 0, 0)

		h, err := globalBytes(data)
		if err != nil {
			return err
		}
		if r, _, e := procSetClipboardData.Call(cfHDrop, h); r == 0 {
			procGlobalFree.Call(h)
			return e
		}

		name, _ := syscall.UTF16PtrFromString("Preferred DropEffect")
		if f, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name))); f != 0 {
			if h, err := globalBytes([]byte{1, 0, 0, 0}); err == nil {
				if r, _, _ := procSetClipboardData.Call(f, h); r == 0 {
					procGlobalFree.Call(h)
				}
			}
		}
		return nil
	})
}
