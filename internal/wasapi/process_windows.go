package wasapi

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	mmdevapi                        = syscall.NewLazyDLL("mmdevapi.dll")
	procActivateAudioInterfaceAsync = mmdevapi.NewProc("ActivateAudioInterfaceAsync")
	procCreateEventW                = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateEventW")

	iidIUnknown          = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidCompletionHandler = guid{0x41D949AB, 0x9862, 0x444A, [8]byte{0x80, 0xF6, 0xC2, 0x61, 0x33, 0x4D, 0xA5, 0xEB}}
	iidAgileObject       = guid{0x94EA2B94, 0xE9CC, 0x49E0, [8]byte{0xC0, 0xFF, 0xEE, 0x64, 0xCA, 0x8F, 0x5B, 0x90}}
)

var ErrNoProcessLoopback = errors.New("per-app audio needs Windows 11")

type activationParams struct {
	activationType uint32
	targetPID      uint32
	mode           uint32
}

type handlerVtbl struct {
	queryInterface    uintptr
	addRef            uintptr
	release           uintptr
	activateCompleted uintptr
}

type handler struct {
	vtbl *handlerVtbl
	done chan uintptr
}

var (
	handlerMu  sync.Mutex
	handlers   = map[uintptr]*handler{}
	sharedVtbl *handlerVtbl
	vtblOnce   sync.Once
)

func guidEq(a *guid, b guid) bool { return *a == b }

// syscall.NewCallback has a hard limit, so one shared vtable
func buildVtbl() {
	sharedVtbl = &handlerVtbl{
		queryInterface: syscall.NewCallback(func(this uintptr, riid *guid, out *uintptr) uintptr {
			if guidEq(riid, iidIUnknown) || guidEq(riid, iidCompletionHandler) || guidEq(riid, iidAgileObject) {
				*out = this
				return 0
			}
			*out = 0
			return 0x80004002
		}),
		addRef:  syscall.NewCallback(func(this uintptr) uintptr { return 1 }),
		release: syscall.NewCallback(func(this uintptr) uintptr { return 1 }),
		activateCompleted: syscall.NewCallback(func(this uintptr, op uintptr) uintptr {
			handlerMu.Lock()
			h := handlers[this]
			handlerMu.Unlock()
			if h != nil {
				select {
				case h.done <- op:
				default:
				}
			}
			return 0
		}),
	}
}

func activateProcessLoopback(pid int, include bool) (com, error) {
	vtblOnce.Do(buildVtbl)
	h := &handler{vtbl: sharedVtbl, done: make(chan uintptr, 1)}
	key := uintptr(unsafe.Pointer(h))
	handlerMu.Lock()
	handlers[key] = h
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		delete(handlers, key)
		handlerMu.Unlock()
		runtime.KeepAlive(h)
	}()

	mode := uint32(1)
	if include {
		mode = 0
	}
	params := activationParams{activationType: 1, targetPID: uint32(pid), mode: mode}
	pv := propVariant{vt: 65, val: uintptr(unsafe.Sizeof(params)), reserved: uintptr(unsafe.Pointer(&params))}
	path, _ := syscall.UTF16PtrFromString(`VAD\Process_Loopback`)
	var op com
	if err := procActivateAudioInterfaceAsync.Find(); err != nil {
		return 0, ErrNoProcessLoopback
	}
	r, _, _ := procActivateAudioInterfaceAsync.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&iidIAudioClient)),
		uintptr(unsafe.Pointer(&pv)), key, uintptr(unsafe.Pointer(&op)))
	runtime.KeepAlive(&params)
	if int32(r) < 0 {
		return 0, ErrNoProcessLoopback
	}
	defer op.release()
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		return 0, errors.New("per-app audio did not respond")
	}
	var hrActivate uintptr
	var unk com
	if r := op.call(3, uintptr(unsafe.Pointer(&hrActivate)), uintptr(unsafe.Pointer(&unk))); int32(r) < 0 {
		return 0, ErrNoProcessLoopback
	}
	if int32(hrActivate) < 0 || unk == 0 {
		return 0, ErrNoProcessLoopback
	}
	return unk, nil
}

func OpenProcess(ctx context.Context, pid int, include bool, out func([]byte)) (*Capture, error) {
	c := &Capture{loopback: true, ready: make(chan error, 1), start: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		coInit()
		defer procCoUninitialize.Call()

		client, err := activateProcessLoopback(pid, include)
		if err != nil {
			c.ready <- err
			return
		}
		defer client.release()
		want := waveFormatEx{FormatTag: waveFormatFloat, Channels: 2, SamplesPerSec: 48000, BitsPerSample: 32, BlockAlign: 8, AvgBytesPerSec: 48000 * 8}
		flags := uintptr(streamFlagsLoopback | 0x00040000 | streamFlagsAutoConvertPCM | streamFlagsSrcDefault)
		if err := hr(client.call(3, 0, flags, 2_000_000, 0, uintptr(unsafe.Pointer(&want)), 0), "IAudioClient.Initialize(process)"); err != nil {
			c.ready <- err
			return
		}
		// required for process loopback even though we poll
		ev, _, _ := procCreateEventW.Call(0, 0, 0, 0)
		if ev != 0 {
			defer syscall.CloseHandle(syscall.Handle(ev))
			client.call(13, ev)
		}
		var capt com
		if err := hr(client.call(14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capt))), "GetService(IAudioCaptureClient)"); err != nil {
			c.ready <- err
			return
		}
		defer capt.release()
		c.Format = Format{SampleFmt: "f32le", SampleRate: 48000, Channels: 2, BlockAlign: 8}
		c.ready <- nil
		select {
		case <-c.start:
		case <-ctx.Done():
			return
		}
		if err := hr(client.call(10), "IAudioClient.Start"); err != nil {
			return
		}
		defer client.call(11)
		c.pump(ctx, capt, out)
	}()
	select {
	case err := <-c.ready:
		if err != nil {
			return nil, err
		}
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
