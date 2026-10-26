package winapi

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegDeleteTreeW   = advapi32.NewProc("RegDeleteTreeW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")

	procGetLogicalDrives      = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW         = kernel32.NewProc("GetDriveTypeW")
	procGetVolumeInformationW = kernel32.NewProc("GetVolumeInformationW")
)

const (
	hkcu         = uintptr(0x80000001)
	hklm         = uintptr(0x80000002)
	keyAllAccess = 0xF003F
	keyRead      = 0x20019
	regSZ        = 1
	regDWORD     = 4
)

func openKey(path string, create bool) (uintptr, error) {
	p, _ := syscall.UTF16PtrFromString(path)
	var h uintptr
	var r uintptr
	if create {
		r, _, _ = procRegCreateKeyExW.Call(hkcu, uintptr(unsafe.Pointer(p)), 0, 0, 0, keyAllAccess, 0, uintptr(unsafe.Pointer(&h)), 0)
	} else {
		r, _, _ = procRegOpenKeyExW.Call(hkcu, uintptr(unsafe.Pointer(p)), 0, keyAllAccess, uintptr(unsafe.Pointer(&h)))
	}
	if r != 0 {
		return 0, syscall.Errno(r)
	}
	return h, nil
}

type RegValue struct {
	Name  string
	Str   string
	Num   uint32
	IsNum bool
}

func RegWrite(path string, values ...RegValue) error {
	h, err := openKey(path, true)
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(h)
	for _, v := range values {
		name, _ := syscall.UTF16PtrFromString(v.Name)
		var r uintptr
		if v.IsNum {
			n := v.Num
			r, _, _ = procRegSetValueExW.Call(h, uintptr(unsafe.Pointer(name)), 0, regDWORD, uintptr(unsafe.Pointer(&n)), 4)
		} else {
			data, _ := syscall.UTF16FromString(v.Str)
			r, _, _ = procRegSetValueExW.Call(h, uintptr(unsafe.Pointer(name)), 0, regSZ, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
		}
		if r != 0 {
			return syscall.Errno(r)
		}
	}
	return nil
}

func RegReadString(path, name string) (string, error) {
	return regRead(hkcu, path, name)
}

func regRead(root uintptr, path, name string) (string, error) {
	p, _ := syscall.UTF16PtrFromString(path)
	var h uintptr
	if r, _, _ := procRegOpenKeyExW.Call(root, uintptr(unsafe.Pointer(p)), 0, keyRead, uintptr(unsafe.Pointer(&h))); r != 0 {
		return "", syscall.Errno(r)
	}
	defer procRegCloseKey.Call(h)
	n, _ := syscall.UTF16PtrFromString(name)
	buf := make([]uint16, 1024)
	size := uint32(len(buf) * 2)
	var typ uint32
	if r, _, _ := procRegQueryValueExW.Call(h, uintptr(unsafe.Pointer(n)), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r != 0 {
		return "", syscall.Errno(r)
	}
	if typ != regSZ {
		return "", errors.New("not a string value")
	}
	return syscall.UTF16ToString(buf[:size/2]), nil
}

func RegDeleteTree(path string) error {
	p, _ := syscall.UTF16PtrFromString(path)
	if r, _, _ := procRegDeleteTreeW.Call(hkcu, uintptr(unsafe.Pointer(p))); r != 0 && r != 2 {
		return syscall.Errno(r)
	}
	parent, leaf := path, ""
	if i := strings.LastIndex(path, `\`); i >= 0 {
		parent, leaf = path[:i], path[i+1:]
	}
	if h, err := openKey(parent, false); err == nil {
		advapi32.NewProc("RegDeleteKeyW").Call(h, uintptr(unsafe.Pointer(mustUTF16(leaf))))
		procRegCloseKey.Call(h)
	}
	return nil
}

func RegDeleteValue(path, name string) {
	if h, err := openKey(path, false); err == nil {
		procRegDeleteValueW.Call(h, uintptr(unsafe.Pointer(mustUTF16(name))))
		procRegCloseKey.Call(h)
	}
}

func mustUTF16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

type Drive struct {
	Root      string `json:"root"`
	Label     string `json:"label"`
	Free      uint64 `json:"free"`
	Total     uint64 `json:"total"`
	System    bool   `json:"system"`
	Removable bool   `json:"removable"`
}

func Drives() []Drive {
	mask, _, _ := procGetLogicalDrives.Call()
	sys := strings.ToUpper(os.Getenv("SystemDrive"))
	var out []Drive
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		typ, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(mustUTF16(root))))
		if typ != 2 && typ != 3 {
			continue
		}
		free, total, err := DiskFree(root)
		if err != nil || total == 0 {
			continue
		}
		label := make([]uint16, 261)
		procGetVolumeInformationW.Call(uintptr(unsafe.Pointer(mustUTF16(root))), uintptr(unsafe.Pointer(&label[0])), 261, 0, 0, 0, 0, 0)
		out = append(out, Drive{
			Root: root, Label: syscall.UTF16ToString(label), Free: free, Total: total,
			System: strings.HasPrefix(root, sys) && sys != "", Removable: typ == 2,
		})
	}
	return out
}

// cheaper than hand-rolling IShellLink
func CreateShortcut(lnk, target, workDir, args string) error {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$s=(New-Object -ComObject WScript.Shell).CreateShortcut(" + q(lnk) + ");" +
		"$s.TargetPath=" + q(target) + ";$s.WorkingDirectory=" + q(workDir) + ";$s.Arguments=" + q(args) + ";" +
		"$s.IconLocation=" + q(target+",0") + ";$s.Save()"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	HideConsole(cmd, false)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}

func CloseProcess(image string) {
	for _, force := range []bool{false, true} {
		args := []string{"/IM", image}
		if force {
			args = append(args, "/F")
		}
		cmd := exec.Command("taskkill.exe", args...)
		HideConsole(cmd, false)
		cmd.Run()
	}
}

// an exe can't delete itself
func RunAfterExit(command string) error {
	cmd := exec.Command("cmd.exe")
	HideConsole(cmd, false)
	// go's arg escaping uses backslashes, cmd doesn't
	cmd.SysProcAttr.CmdLine = `cmd.exe /C "ping 127.0.0.1 -n 3 >nul & ` + command + `"`
	return cmd.Start()
}

type GPU struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Driver string `json:"driver"`
}

const displayClass = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`

// no admin, no wmi
func GPUs() []GPU {
	var out []GPU
	for i := 0; i < 10; i++ {
		key := displayClass + `\` + leftPad(i)
		name, err := regRead(hklm, key, "DriverDesc")
		if err != nil || name == "" {
			continue
		}
		ver, _ := regRead(hklm, key, "DriverVersion")
		lower := strings.ToLower(name)
		g := GPU{Name: name, Driver: ver}
		switch {
		case strings.Contains(lower, "nvidia"):
			g.Vendor = "nvidia"
			g.Driver = nvidiaVersion(ver)
		case strings.Contains(lower, "amd") || strings.Contains(lower, "radeon"):
			g.Vendor = "amd"
		case strings.Contains(lower, "intel"):
			g.Vendor = "intel"
		default:
			continue
		}
		out = append(out, g)
	}
	return out
}

func leftPad(i int) string {
	s := "000" + string(rune('0'+i))
	return s[len(s)-4:]
}

// 32.0.15.6094 -> 560.94
func nvidiaVersion(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) != 4 {
		return v
	}
	d := parts[2] + parts[3]
	if len(d) < 5 {
		return v
	}
	d = d[len(d)-5:]
	return strings.TrimLeft(d[:3], "0") + "." + d[3:]
}

type osVersionInfo struct {
	size, major, minor, build, platform uint32
	csd                                 [128]uint16
}

// GetVersionEx lies without a manifest entry
func WindowsVersion() string {
	var v osVersionInfo
	v.size = uint32(unsafe.Sizeof(v))
	r, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&v)))
	if r != 0 {
		return "unknown"
	}
	name := "Windows 10"
	if v.major == 10 && v.build >= 22000 {
		name = "Windows 11"
	}
	return name + " build " + strconv.Itoa(int(v.build))
}
