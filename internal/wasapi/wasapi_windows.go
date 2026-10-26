package wasapi

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

type guid struct {
	d1 uint32
	d2 uint16
	d3 uint16
	d4 [8]byte
}

var (
	clsidMMDeviceEnumerator = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioClient         = guid{0x1CB9AD4C, 0xDBFA, 0x4C32, [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioCaptureClient  = guid{0xC8ADBD64, 0xE71E, 0x48A0, [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
	pkeyDeviceFriendlyName  = propertyKey{guid{0xA45C254E, 0xDF1C, 0x4EFD, [8]byte{0x80, 0x20, 0x67, 0xD1, 0x46, 0xA8, 0x50, 0xE0}}, 14}
)

type propertyKey struct {
	fmtid guid
	pid   uint32
}

type propVariant struct {
	vt       uint16
	r1       uint16
	r2       uint16
	r3       uint16
	val      uintptr
	reserved uintptr
}

const (
	eRender           = 0
	eCapture          = 1
	eConsole          = 0
	deviceStateActive = 1
	clsctxAll         = 0x17

	streamFlagsLoopback       = 0x00020000
	streamFlagsAutoConvertPCM = 0x80000000
	streamFlagsSrcDefault     = 0x08000000
	bufferFlagsSilent         = 0x2

	waveFormatPCM    = 1
	waveFormatFloat  = 3
	waveFormatExtTag = 0xFFFE
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

type waveFormatExtensible struct {
	waveFormatEx
	ValidBits   uint16
	ChannelMask uint32
	SubFormat   guid
}

// six interfaces don't justify a com library
type com uintptr

func (c com) call(slot int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(c))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(c)}, args...)...)
	return r
}

func (c com) release() {
	if c != 0 {
		c.call(2)
	}
}

func hr(r uintptr, what string) error {
	if int32(r) < 0 {
		return fmt.Errorf("%s failed: 0x%08X", what, uint32(r))
	}
	return nil
}

func coInit() {
	procCoInitializeEx.Call(0, 0)
}

func enumerator() (com, error) {
	var e com
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&e)))
	return e, hr(r, "CoCreateInstance(MMDeviceEnumerator)")
}

func lpwstr(p uintptr) string {
	if p == 0 {
		return ""
	}
	var s []uint16
	for i := uintptr(0); ; i += 2 {
		ch := *(*uint16)(unsafe.Pointer(p + i))
		if ch == 0 {
			break
		}
		s = append(s, ch)
	}
	return syscall.UTF16ToString(s)
}

type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

func deviceID(dev com) string {
	var p uintptr
	if dev.call(5, uintptr(unsafe.Pointer(&p))) != 0 {
		return ""
	}
	s := lpwstr(p)
	procCoTaskMemFree.Call(p)
	return s
}

func deviceName(dev com) string {
	var store com
	if dev.call(4, 0, uintptr(unsafe.Pointer(&store))) != 0 {
		return ""
	}
	defer store.release()
	var pv propVariant
	if store.call(5, uintptr(unsafe.Pointer(&pkeyDeviceFriendlyName)), uintptr(unsafe.Pointer(&pv))) != 0 {
		return ""
	}
	name := ""
	if pv.vt == 31 {
		name = lpwstr(pv.val)
	}
	procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	return name
}

func List(capture bool) ([]Device, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	coInit()
	defer procCoUninitialize.Call()

	en, err := enumerator()
	if err != nil {
		return nil, err
	}
	defer en.release()
	flow := uintptr(eRender)
	if capture {
		flow = eCapture
	}
	defID := ""
	var def com
	if en.call(4, flow, eConsole, uintptr(unsafe.Pointer(&def))) == 0 {
		defID = deviceID(def)
		def.release()
	}
	var coll com
	if err := hr(en.call(3, flow, deviceStateActive, uintptr(unsafe.Pointer(&coll))), "EnumAudioEndpoints"); err != nil {
		return nil, err
	}
	defer coll.release()
	var n uint32
	coll.call(3, uintptr(unsafe.Pointer(&n)))
	out := make([]Device, 0, n)
	for i := uint32(0); i < n; i++ {
		var dev com
		if coll.call(4, uintptr(i), uintptr(unsafe.Pointer(&dev))) != 0 {
			continue
		}
		d := Device{ID: deviceID(dev), Name: deviceName(dev)}
		d.IsDefault = d.ID == defID
		out = append(out, d)
		dev.release()
	}
	return out, nil
}

type Format struct {
	SampleFmt  string
	SampleRate int
	Channels   int
	BlockAlign int
}

type Capture struct {
	Format   Format
	loopback bool
	devID    string
	ready    chan error
	start    chan struct{}
}

// com is thread bound
func Open(ctx context.Context, deviceID string, loopback bool, out func([]byte)) (*Capture, error) {
	c := &Capture{loopback: loopback, devID: deviceID, ready: make(chan error, 1), start: make(chan struct{})}
	go c.thread(ctx, out)
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

func (c *Capture) Start() { close(c.start) }

func (c *Capture) thread(ctx context.Context, out func([]byte)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	coInit()
	defer procCoUninitialize.Call()

	client, capt, err := c.init()
	if err != nil {
		c.ready <- err
		return
	}
	defer client.release()
	defer capt.release()
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
}

func (c *Capture) init() (com, com, error) {
	en, err := enumerator()
	if err != nil {
		return 0, 0, err
	}
	defer en.release()

	flow := uintptr(eCapture)
	if c.loopback {
		flow = eRender
	}
	var dev com
	if c.devID != "" {
		id, _ := syscall.UTF16PtrFromString(c.devID)
		if en.call(5, uintptr(unsafe.Pointer(id)), uintptr(unsafe.Pointer(&dev))) != 0 {
			dev = 0
		}
	}
	if dev == 0 {
		if err := hr(en.call(4, flow, eConsole, uintptr(unsafe.Pointer(&dev))), "GetDefaultAudioEndpoint"); err != nil {
			return 0, 0, err
		}
	}
	defer dev.release()

	var client com
	if err := hr(dev.call(3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))), "IMMDevice.Activate"); err != nil {
		return 0, 0, err
	}

	flags := uintptr(0)
	if c.loopback {
		flags |= streamFlagsLoopback
	}
	const bufferDuration = 2_000_000 // 200ms in 100ns units

	want := waveFormatEx{FormatTag: waveFormatFloat, Channels: 2, SamplesPerSec: 48000, BitsPerSample: 32, BlockAlign: 8, AvgBytesPerSec: 48000 * 8}
	r := client.call(3, 0, flags|streamFlagsAutoConvertPCM|streamFlagsSrcDefault, bufferDuration, 0, uintptr(unsafe.Pointer(&want)), 0)
	if int32(r) >= 0 {
		c.Format = Format{SampleFmt: "f32le", SampleRate: 48000, Channels: 2, BlockAlign: 8}
	} else {
		// autoconvert refused
		client.release()
		client = 0
		if err := hr(dev.call(3, uintptr(unsafe.Pointer(&iidIAudioClient)), clsctxAll, 0, uintptr(unsafe.Pointer(&client))), "IMMDevice.Activate"); err != nil {
			return 0, 0, err
		}
		var mix uintptr
		if err := hr(client.call(8, uintptr(unsafe.Pointer(&mix))), "GetMixFormat"); err != nil {
			client.release()
			return 0, 0, err
		}
		f, ferr := describe(mix)
		r = client.call(3, 0, flags, bufferDuration, 0, mix, 0)
		procCoTaskMemFree.Call(mix)
		if ferr != nil {
			client.release()
			return 0, 0, ferr
		}
		if err := hr(r, "IAudioClient.Initialize"); err != nil {
			client.release()
			return 0, 0, err
		}
		c.Format = f
	}

	var capt com
	if err := hr(client.call(14, uintptr(unsafe.Pointer(&iidIAudioCaptureClient)), uintptr(unsafe.Pointer(&capt))), "GetService(IAudioCaptureClient)"); err != nil {
		client.release()
		return 0, 0, err
	}
	return client, capt, nil
}

func describe(p uintptr) (Format, error) {
	wf := (*waveFormatEx)(unsafe.Pointer(p))
	tag := wf.FormatTag
	bits := wf.BitsPerSample
	if tag == waveFormatExtTag && wf.CbSize >= 22 {
		ext := (*waveFormatExtensible)(unsafe.Pointer(p))
		tag = uint16(ext.SubFormat.d1)
	}
	f := Format{SampleRate: int(wf.SamplesPerSec), Channels: int(wf.Channels), BlockAlign: int(wf.BlockAlign)}
	switch {
	case tag == waveFormatFloat && bits == 32:
		f.SampleFmt = "f32le"
	case tag == waveFormatPCM && bits == 16:
		f.SampleFmt = "s16le"
	case tag == waveFormatPCM && bits == 24:
		f.SampleFmt = "s24le"
	case tag == waveFormatPCM && bits == 32:
		f.SampleFmt = "s32le"
	default:
		return f, errors.New("unsupported device mix format")
	}
	return f, nil
}

// loopback goes quiet during silence, pad to stay on the clock
func (c *Capture) pump(ctx context.Context, capt com, out func([]byte)) {
	rate := float64(c.Format.SampleRate)
	ba := c.Format.BlockAlign
	start := time.Now()
	var written int64
	silence := make([]byte, ba*int(rate/10))
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		got := false
		for {
			var next uint32
			if capt.call(5, uintptr(unsafe.Pointer(&next))) != 0 || next == 0 {
				break
			}
			var data uintptr
			var frames, flags uint32
			if capt.call(3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), 0, 0) != 0 {
				break
			}
			n := int(frames) * ba
			if n > 0 {
				buf := make([]byte, n)
				if flags&bufferFlagsSilent == 0 && data != 0 {
					copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(data)), n))
				}
				expected := int64(time.Since(start).Seconds() * rate)
				if written-expected < int64(rate/5) {
					out(buf)
					written += int64(frames)
				}
				got = true
			}
			capt.call(4, uintptr(frames))
		}
		if got {
			continue
		}
		expected := int64(time.Since(start).Seconds() * rate)
		missing := expected - written - int64(rate*0.02)
		for missing > 0 {
			f := missing
			if max := int64(len(silence) / ba); f > max {
				f = max
			}
			out(append([]byte(nil), silence[:int(f)*ba]...))
			written += f
			missing -= f
		}
	}
}
