package winapi

import (
	"encoding/base64"
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

var (
	crypt32 = syscall.NewLazyDLL("crypt32.dll")

	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procLocalFree                = kernel32.NewProc("LocalFree")
	procCryptProtectData         = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData       = crypt32.NewProc("CryptUnprotectData")
)

type processEntry struct {
	size          uint32
	usage         uint32
	pid           uint32
	defaultHeapID uintptr
	moduleID      uint32
	threads       uint32
	parentPID     uint32
	priClassBase  int32
	flags         uint32
	exeFile       [260]uint16
}

type Proc struct {
	PID    int
	Parent int
	Exe    string
}

func Processes() []Proc {
	snap, _, _ := procCreateToolhelp32Snapshot.Call(0x2, 0)
	if syscall.Handle(snap) == syscall.InvalidHandle {
		return nil
	}
	defer syscall.CloseHandle(syscall.Handle(snap))
	var e processEntry
	e.size = uint32(unsafe.Sizeof(e))
	var out []Proc
	r, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&e)))
	for r != 0 {
		out = append(out, Proc{PID: int(e.pid), Parent: int(e.parentPID), Exe: syscall.UTF16ToString(e.exeFile[:])})
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&e)))
	}
	return out
}

// process loopback wants the tree root
func RootProcess(names ...string) int {
	procs := Processes()
	match := func(exe string) bool {
		for _, n := range names {
			if strings.EqualFold(exe, n) {
				return true
			}
		}
		return false
	}
	byPID := map[int]Proc{}
	for _, p := range procs {
		byPID[p.PID] = p
	}
	for _, p := range procs {
		if !match(p.Exe) {
			continue
		}
		if parent, ok := byPID[p.Parent]; !ok || !match(parent.Exe) {
			return p.PID
		}
	}
	return 0
}

type dataBlob struct {
	size uint32
	data *byte
}

func Protect(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	in := dataBlob{size: uint32(len(plain)), data: &[]byte(plain)[0]}
	var out dataBlob
	if r, _, e := procCryptProtectData.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0x1, uintptr(unsafe.Pointer(&out))); r == 0 {
		return "", e
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	b := unsafe.Slice(out.data, out.size)
	return "dpapi:" + base64.StdEncoding.EncodeToString(b), nil
}

func Unprotect(stored string) (string, error) {
	if !strings.HasPrefix(stored, "dpapi:") {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "dpapi:"))
	if err != nil || len(raw) == 0 {
		return "", errors.New("stored key is damaged")
	}
	in := dataBlob{size: uint32(len(raw)), data: &raw[0]}
	var out dataBlob
	if r, _, e := procCryptUnprotectData.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0x1, uintptr(unsafe.Pointer(&out))); r == 0 {
		return "", e
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return string(unsafe.Slice(out.data, out.size)), nil
}
