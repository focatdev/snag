package tsbuf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	warmup     = clock / 2
	PacketSize = 188
	ptsWrap    = int64(1) << 33
	clock      = 90000
)

type segment struct {
	data   []byte
	file   string
	size   int
	pts    int64
	wall   time.Time
	pinned int
}

type Ring struct {
	mu       sync.Mutex
	maxDur   time.Duration
	segs     []*segment
	cur      *segment
	bytes    int
	pat      []byte
	pmt      []byte
	pmtPID   int
	vidPID   int
	vidType  byte
	audPIDs  map[int]bool
	lastPTS  int64
	firstPTS int64
	marks    []int64
	seenKey  bool
	vidAt    time.Time
	disk     string
	seq      int
	orphans  []*segment
}

func (r *Ring) SetDisk(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disk = dir
	if dir != "" {
		os.MkdirAll(dir, 0o755)
		old, _ := filepath.Glob(filepath.Join(dir, "seg-*.ts"))
		for _, f := range old {
			os.Remove(f)
		}
	}
}

func (r *Ring) drop(s *segment) {
	if s.file == "" {
		return
	}
	if s.pinned > 0 {
		r.orphans = append(r.orphans, s)
		return
	}
	os.Remove(s.file)
}

func NewRing(maxDur time.Duration) *Ring {
	return &Ring{maxDur: maxDur, pmtPID: -1, vidPID: -1, lastPTS: -1, firstPTS: -1}
}

func (r *Ring) SetMaxDuration(d time.Duration) {
	r.mu.Lock()
	r.maxDur = d
	r.trim()
	r.mu.Unlock()
}

func (r *Ring) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.segs {
		r.drop(s)
	}
	r.segs, r.cur, r.bytes = nil, nil, 0
	r.pat, r.pmt = nil, nil
	r.pmtPID, r.vidPID, r.vidType = -1, -1, 0
	r.lastPTS, r.firstPTS = -1, -1
	r.audPIDs = nil
	r.marks = nil
	r.seenKey = false
	r.vidAt = time.Time{}
}

// how long since ffmpeg last sent a video frame, zero before the first one
func (r *Ring) VideoAge() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.vidAt.IsZero() {
		return 0
	}
	return time.Since(r.vidAt)
}

// a restarted pipe can leave a partial packet
func (r *Ring) Feed(rd io.Reader) error {
	br := bufio.NewReaderSize(rd, PacketSize*512)
	pkt := make([]byte, PacketSize)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		if b != 0x47 {
			continue
		}
		pkt[0] = b
		if _, err := io.ReadFull(br, pkt[1:]); err != nil {
			return err
		}
		if next, err := br.Peek(1); err == nil && next[0] != 0x47 {
			continue
		}
		r.packet(pkt)
	}
}

func (r *Ring) packet(p []byte) {
	pid := int(p[1]&0x1f)<<8 | int(p[2])
	pusi := p[1]&0x40 != 0
	afc := (p[3] >> 4) & 3
	off := 4
	rai := false
	if afc&2 != 0 {
		alen := int(p[4])
		if alen > 0 && p[5]&0x40 != 0 {
			rai = true
		}
		off = 5 + alen
	}
	var payload []byte
	if afc&1 != 0 && off < PacketSize {
		payload = p[off:]
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	switch {
	case pid == 0 && pusi && payload != nil:
		if prog := parsePAT(payload); prog >= 0 {
			r.pmtPID = prog
			r.pat = append(r.pat[:0], p...)
		}
	case pid == r.pmtPID && pusi && payload != nil:
		if vp, vt := parsePMT(payload); vp >= 0 {
			r.vidPID, r.vidType = vp, vt
			r.pmt = append(r.pmt[:0], p...)
			r.audPIDs = audioPIDs(payload)
		}
	}

	if pid == r.vidPID && pusi && payload != nil {
		r.vidAt = time.Now()
		pts := parsePTS(payload)
		if pts >= 0 {
			r.lastPTS = pts
			if r.firstPTS < 0 {
				r.firstPTS = pts
			}
		}
		// skip the startup junk: ddagrab's probe frame, audio before all pipes connect
		warm := r.cur != nil || pts < 0 || r.firstPTS < 0 || ptsDiff(pts, r.firstPTS) >= warmup
		if warm && (rai || isKeyframe(payload, r.vidType)) {
			r.startSegment(pts)
		}
	}
	if r.cur == nil {
		return
	}
	r.cur.data = append(r.cur.data, p...)
	r.bytes += PacketSize
}

func (r *Ring) startSegment(pts int64) {
	capHint := 1 << 20
	if r.cur != nil {
		done := r.cur
		capHint = cap(done.data)
		done.size = len(done.data)
		if r.disk != "" {
			r.seq++
			name := filepath.Join(r.disk, fmt.Sprintf("seg-%08d.ts", r.seq))
			if err := os.WriteFile(name, done.data, 0o644); err == nil {
				done.file, done.data = name, nil
			}
		}
		r.segs = append(r.segs, done)
	}
	r.cur = &segment{data: make([]byte, 0, capHint), pts: pts, wall: time.Now()}
	r.seenKey = true
	r.trim()
}

func (r *Ring) trim() {
	if r.cur == nil {
		return
	}
	limit := int64(r.maxDur.Seconds() * clock)
	for len(r.segs) > 1 {
		second := r.segs[1]
		if r.span(second, r.cur) < limit {
			break
		}
		r.bytes -= r.segs[0].size
		r.drop(r.segs[0])
		r.segs[0] = nil
		r.segs = r.segs[1:]
	}
	if len(r.segs) > 0 {
		oldest := r.segs[0].pts
		kept := r.marks[:0]
		for _, m := range r.marks {
			if oldest < 0 || ptsDiff(m, oldest) >= 0 {
				kept = append(kept, m)
			}
		}
		r.marks = kept
	}
}

// some pes headers have no pts
func (r *Ring) span(a, b *segment) int64 {
	if a.pts >= 0 && b.pts >= 0 {
		return ptsDiff(b.pts, a.pts)
	}
	return int64(b.wall.Sub(a.wall).Seconds() * clock)
}

func ptsDiff(a, b int64) int64 {
	d := (a - b) % ptsWrap
	if d < 0 {
		d += ptsWrap
	}
	if d > ptsWrap/2 {
		d -= ptsWrap
	}
	return d
}

func (r *Ring) Buffered() (float64, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == nil {
		return 0, r.bytes
	}
	first := r.cur
	if len(r.segs) > 0 {
		first = r.segs[0]
	}
	if first.pts >= 0 && r.lastPTS >= 0 {
		return float64(ptsDiff(r.lastPTS, first.pts)) / clock, r.bytes
	}
	return time.Since(first.wall).Seconds(), r.bytes
}

func (r *Ring) AddMark() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastPTS < 0 {
		return false
	}
	r.marks = append(r.marks, r.lastPTS)
	return true
}

var ErrEmpty = errors.New("buffer is empty")

type SnapInfo struct {
	Marks    []float64
	StartPTS int64
	EndPTS   int64
	Seconds  float64
}

func (r *Ring) Contains(pts int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pts < 0 || r.cur == nil {
		return false
	}
	first := r.cur
	if len(r.segs) > 0 {
		first = r.segs[0]
	}
	return first.pts >= 0 && ptsDiff(pts, first.pts) >= 0
}

// write outside the lock, a slow disk must not stall capture
func (r *Ring) Snapshot(w io.Writer, seconds float64, since int64) (SnapInfo, error) {
	r.mu.Lock()
	all := make([]*segment, 0, len(r.segs)+1)
	all = append(all, r.segs...)
	if r.cur != nil {
		all = append(all, r.cur)
	}
	if len(all) == 0 || len(r.pat) == 0 || len(r.pmt) == 0 {
		r.mu.Unlock()
		return SnapInfo{}, ErrEmpty
	}
	end := r.lastPTS
	limit := int64(seconds * clock)
	start := 0
	for i := len(all) - 1; i >= 0; i-- {
		start = i
		if all[i].pts < 0 || end < 0 {
			continue
		}
		if since >= 0 {
			if ptsDiff(since, all[i].pts) >= 0 {
				break
			}
		} else if ptsDiff(end, all[i].pts) >= limit {
			break
		}
	}
	type chunk struct {
		data []byte
		seg  *segment
	}
	chunks := make([]chunk, 0, len(all)-start+2)
	chunks = append(chunks, chunk{data: append([]byte(nil), r.pat...)}, chunk{data: append([]byte(nil), r.pmt...)})
	for _, s := range all[start:] {
		if s.file != "" {
			s.pinned++
			chunks = append(chunks, chunk{seg: s})
			continue
		}
		data := s.data[:len(s.data):len(s.data)]
		// half an aac frame at the end kills chromium's whole pipeline
		if s == r.cur {
			data = completeFrames(data, r.vidPID)
		}
		chunks = append(chunks, chunk{data: data})
	}
	info := SnapInfo{StartPTS: all[start].pts, EndPTS: end}
	trim := newAVTrim(r.audPIDs, info.StartPTS, end)
	if info.StartPTS >= 0 && end >= 0 {
		info.Seconds = float64(ptsDiff(end, info.StartPTS)) / clock
	}
	for _, m := range r.marks {
		if info.StartPTS >= 0 {
			if d := ptsDiff(m, info.StartPTS); d >= 0 {
				info.Marks = append(info.Marks, float64(d)/clock)
			}
		}
	}
	r.mu.Unlock()

	var werr error
	for _, c := range chunks {
		if werr != nil {
			break
		}
		data := c.data
		if c.seg != nil {
			b, err := os.ReadFile(c.seg.file)
			if err != nil {
				werr = err
				break
			}
			data = b
		}
		_, werr = w.Write(trim.filter(data))
	}

	r.mu.Lock()
	for _, c := range chunks {
		if c.seg != nil {
			c.seg.pinned--
		}
	}
	kept := r.orphans[:0]
	for _, o := range r.orphans {
		if o.pinned > 0 {
			kept = append(kept, o)
		} else {
			os.Remove(o.file)
		}
	}
	r.orphans = kept
	r.mu.Unlock()
	return info, werr
}

func parsePAT(payload []byte) int {
	if len(payload) < 1 {
		return -1
	}
	ptr := int(payload[0])
	t := payload[1+ptr:]
	if len(t) < 8 || t[0] != 0x00 {
		return -1
	}
	secLen := int(t[1]&0x0f)<<8 | int(t[2])
	end := 3 + secLen - 4
	if end > len(t) {
		end = len(t)
	}
	for i := 8; i+4 <= end; i += 4 {
		prog := int(t[i])<<8 | int(t[i+1])
		pid := int(t[i+2]&0x1f)<<8 | int(t[i+3])
		if prog != 0 {
			return pid
		}
	}
	return -1
}

func parsePMT(payload []byte) (int, byte) {
	if len(payload) < 1 {
		return -1, 0
	}
	ptr := int(payload[0])
	t := payload[1+ptr:]
	if len(t) < 12 || t[0] != 0x02 {
		return -1, 0
	}
	secLen := int(t[1]&0x0f)<<8 | int(t[2])
	end := 3 + secLen - 4
	if end > len(t) {
		end = len(t)
	}
	progInfo := int(t[10]&0x0f)<<8 | int(t[11])
	i := 12 + progInfo
	for i+5 <= end {
		st := t[i]
		pid := int(t[i+1]&0x1f)<<8 | int(t[i+2])
		esLen := int(t[i+3]&0x0f)<<8 | int(t[i+4])
		switch st {
		case 0x1b, 0x24, 0x02, 0x10:
			return pid, st
		}
		i += 5 + esLen
	}
	return -1, 0
}

func parsePTS(payload []byte) int64 {
	if len(payload) < 14 || payload[0] != 0 || payload[1] != 0 || payload[2] != 1 {
		return -1
	}
	if payload[7]&0x80 == 0 {
		return -1
	}
	b := payload[9:14]
	return int64(b[0]>>1&0x07)<<30 | int64(b[1])<<22 | int64(b[2]>>1)<<15 | int64(b[3])<<7 | int64(b[4]>>1)
}

// not every muxer sets the random access flag
func isKeyframe(payload []byte, streamType byte) bool {
	if len(payload) < 9 || 9+int(payload[8]) >= len(payload) {
		return false
	}
	es := payload[9+int(payload[8]):]
	for i := 0; i+4 < len(es); i++ {
		if es[i] != 0 || es[i+1] != 0 || es[i+2] != 1 {
			continue
		}
		nal := es[i+3]
		switch streamType {
		case 0x1b:
			t := nal & 0x1f
			if t == 5 || t == 7 {
				return true
			}
		case 0x24:
			t := (nal >> 1) & 0x3f
			if (t >= 16 && t <= 21) || t == 32 || t == 33 {
				return true
			}
		}
		i += 3
	}
	return false
}

func audioPIDs(payload []byte) map[int]bool {
	out := map[int]bool{}
	if len(payload) < 1 {
		return out
	}
	ptr := int(payload[0])
	if 1+ptr >= len(payload) {
		return out
	}
	t := payload[1+ptr:]
	if len(t) < 12 || t[0] != 0x02 {
		return out
	}
	secLen := int(t[1]&0x0f)<<8 | int(t[2])
	end := 3 + secLen - 4
	if end > len(t) {
		end = len(t)
	}
	i := 12 + (int(t[10]&0x0f)<<8 | int(t[11]))
	for i+5 <= end {
		st := t[i]
		pid := int(t[i+1]&0x1f)<<8 | int(t[i+2])
		switch st {
		case 0x0f, 0x11, 0x03, 0x04, 0x81, 0x87:
			out[pid] = true
		}
		i += 5 + (int(t[i+3]&0x0f)<<8 | int(t[i+4]))
	}
	return out
}

// audio outside the video makes the mp4 longer than the video, by hours if the video stalled
type avTrim struct {
	start, end int64
	aud        map[int]bool
	waiting    map[int]bool
	past       map[int]bool
}

func newAVTrim(aud map[int]bool, start, end int64) *avTrim {
	t := &avTrim{start: start, end: end, aud: aud, waiting: map[int]bool{}, past: map[int]bool{}}
	if start < 0 {
		t.aud = nil
		return t
	}
	for pid := range aud {
		t.waiting[pid] = true
	}
	return t
}

func pesStartPTS(p []byte) int64 {
	afc := (p[3] >> 4) & 3
	o := 4
	if afc&2 != 0 {
		o = 5 + int(p[4])
	}
	if afc&1 == 0 || o >= PacketSize {
		return -1
	}
	return parsePTS(p[o:])
}

func (t *avTrim) filter(data []byte) []byte {
	if len(t.aud) == 0 {
		return data
	}
	out := data[:0:0]
	for off := 0; off+PacketSize <= len(data); off += PacketSize {
		p := data[off : off+PacketSize]
		pid := int(p[1]&0x1f)<<8 | int(p[2])
		if t.aud[pid] {
			if t.past[pid] {
				continue
			}
			if p[1]&0x40 == 0 {
				if t.waiting[pid] {
					continue
				}
			} else {
				pts := pesStartPTS(p)
				if t.waiting[pid] {
					if pts < 0 || ptsDiff(pts, t.start) < 0 {
						continue
					}
					delete(t.waiting, pid)
				}
				if pts >= 0 && t.end >= 0 && ptsDiff(pts, t.end) > clock {
					t.past[pid] = true
					continue
				}
			}
		}
		out = append(out, p...)
	}
	return out
}

func completeFrames(data []byte, vidPID int) []byte {
	last := map[int]int{}
	first := map[int]int{}
	for off := 0; off+PacketSize <= len(data); off += PacketSize {
		p := data[off:]
		if p[1]&0x40 == 0 {
			continue
		}
		pid := int(p[1]&0x1f)<<8 | int(p[2])
		if _, ok := first[pid]; !ok {
			first[pid] = off
		}
		last[pid] = off
	}
	// keep a lone keyframe
	if last[vidPID] == first[vidPID] {
		delete(last, vidPID)
	}
	out := make([]byte, 0, len(data))
	for off := 0; off+PacketSize <= len(data); off += PacketSize {
		p := data[off : off+PacketSize]
		pid := int(p[1]&0x1f)<<8 | int(p[2])
		if cut, ok := last[pid]; ok && off >= cut {
			continue
		}
		out = append(out, p...)
	}
	return out
}
