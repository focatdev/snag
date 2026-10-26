package winapi

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	procCreateNamedPipeW    = kernel32.NewProc("CreateNamedPipeW")
	procConnectNamedPipe    = kernel32.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe = kernel32.NewProc("DisconnectNamedPipe")
)

const errorPipeConnected = 535

type Pipe struct {
	Name      string
	h         syscall.Handle
	mu        sync.Mutex
	connected bool
	closed    bool
}

func NewPipe(name string) (*Pipe, error) {
	full := `\\.\pipe\` + name
	p, err := syscall.UTF16PtrFromString(full)
	if err != nil {
		return nil, err
	}
	// ffmpeg is slow to start reading
	h, _, e := procCreateNamedPipeW.Call(uintptr(unsafe.Pointer(p)), 0x2, 0, 1, 4<<20, 0, 0, 0)
	if syscall.Handle(h) == syscall.InvalidHandle {
		return nil, e
	}
	return &Pipe{Name: full, h: syscall.Handle(h)}, nil
}

func (p *Pipe) Accept() error {
	r, _, e := procConnectNamedPipe.Call(uintptr(p.h), 0)
	if r == 0 && e != syscall.Errno(errorPipeConnected) {
		return e
	}
	p.mu.Lock()
	p.connected = true
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return syscall.EINVAL
	}
	return nil
}

func (p *Pipe) Write(b []byte) (int, error) {
	var n uint32
	err := syscall.WriteFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *Pipe) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	connected := p.connected
	p.mu.Unlock()
	if !connected {
		if name, err := syscall.UTF16PtrFromString(p.Name); err == nil {
			if c, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0); err == nil {
				syscall.CloseHandle(c)
			}
		}
	}
	procDisconnectNamedPipe.Call(uintptr(p.h))
	return syscall.CloseHandle(p.h)
}
