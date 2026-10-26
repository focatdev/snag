package library

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"snag/internal/config"
	"snag/internal/ffmpeg"
	"snag/internal/winapi"
)

type Clip struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	Title      string    `json:"title"`
	Game       string    `json:"game"`
	CreatedAt  time.Time `json:"createdAt"`
	Duration   float64   `json:"duration"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	FPS        float64   `json:"fps"`
	Size       int64     `json:"size"`
	Codec      string    `json:"codec"`
	Tracks     []string  `json:"tracks"`
	Markers    []float64 `json:"markers"`
	Favorite   bool      `json:"favorite"`
	Tags       []string  `json:"tags"`
	Thumb      bool      `json:"thumb"`
	Sprite     bool      `json:"sprite"`
	Frames     int       `json:"spriteFrames"`
	Export     bool      `json:"export"`
	Probed     bool      `json:"probed"`
	Shrunk     bool      `json:"shrunk"`
	OrigSize   int64     `json:"origSize"`
	AITitled   bool      `json:"aiTitled"`
	Transcript string    `json:"transcript"`
	Link       string    `json:"link,omitempty"`
	DeleteLink string    `json:"deleteLink,omitempty"`
}

type Library struct {
	mu       sync.Mutex
	dataDir  string
	path     string
	clips    map[string]*Clip
	tools    func() ffmpeg.Tools
	onChange func()
	work     chan string
	saveT    *time.Timer
}

func Open(dataDir string, tools func() ffmpeg.Tools, onChange func()) *Library {
	l := &Library{
		dataDir:  dataDir,
		path:     filepath.Join(dataDir, "library.json"),
		clips:    map[string]*Clip{},
		tools:    tools,
		onChange: onChange,
		work:     make(chan string, 4096),
	}
	if b, err := os.ReadFile(l.path); err == nil {
		var list []*Clip
		if json.Unmarshal(b, &list) == nil {
			for _, c := range list {
				l.clips[c.ID] = c
			}
		}
	}
	os.MkdirAll(l.ThumbDir(), 0o755)
	go l.worker()
	return l
}

func (l *Library) ThumbDir() string { return filepath.Join(l.dataDir, "thumbs") }

func (l *Library) ThumbPath(id string) string  { return filepath.Join(l.ThumbDir(), id+".jpg") }
func (l *Library) SpritePath(id string) string { return filepath.Join(l.ThumbDir(), id+"_sprite.jpg") }
func (l *Library) wavePath(id string, track int) string {
	return filepath.Join(l.ThumbDir(), fmt.Sprintf("%s_wave%d.bin", id, track))
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (l *Library) changed() {
	l.mu.Lock()
	if l.saveT != nil {
		l.saveT.Stop()
	}
	l.saveT = time.AfterFunc(300*time.Millisecond, l.persist)
	l.mu.Unlock()
	if l.onChange != nil {
		l.onChange()
	}
}

func (l *Library) persist() {
	l.mu.Lock()
	list := l.sortedLocked()
	b, err := json.MarshalIndent(list, "", " ")
	l.mu.Unlock()
	if err == nil {
		config.WriteFileAtomic(l.path, b)
	}
}

func (l *Library) sortedLocked() []Clip {
	out := make([]Clip, 0, len(l.clips))
	for _, c := range l.clips {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (l *Library) List() []Clip {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sortedLocked()
}

func (l *Library) Get(id string) (Clip, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.clips[id]
	if !ok {
		return Clip{}, false
	}
	return *c, true
}

type NewClip struct {
	Path    string
	Game    string
	Markers []float64
	Tracks  []string
	Export  bool
}

func (l *Library) Add(n NewClip) Clip {
	st, _ := os.Stat(n.Path)
	c := &Clip{
		ID:        newID(),
		Path:      n.Path,
		Title:     strings.TrimSuffix(filepath.Base(n.Path), filepath.Ext(n.Path)),
		Game:      n.Game,
		CreatedAt: time.Now(),
		Markers:   n.Markers,
		Tracks:    n.Tracks,
		Export:    n.Export,
		Tags:      []string{},
	}
	if st != nil {
		c.Size = st.Size()
		c.CreatedAt = st.ModTime()
	}
	l.probe(c)
	l.mu.Lock()
	l.clips[c.ID] = c
	out := *c
	l.mu.Unlock()
	l.changed()
	l.enqueue(c.ID)
	return out
}

func (l *Library) probe(c *Clip) {
	t := l.tools()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mi, err := ffmpeg.Probe(ctx, t.FFprobe, c.Path)
	if err != nil {
		return
	}
	c.Duration, c.Width, c.Height, c.FPS, c.Codec = mi.Duration, mi.Width, mi.Height, mi.FPS, mi.VideoCodec
	if len(c.Tracks) == 0 || len(c.Tracks) != len(mi.AudioTracks) {
		c.Tracks = mi.AudioTracks
	}
	if mi.Size > 0 {
		c.Size = mi.Size
	}
	c.Probed = true
}

func (l *Library) enqueue(id string) {
	select {
	case l.work <- id:
	default:
	}
}

// one at a time, a rescan of 500 clips shouldn't peg the cpu
func (l *Library) worker() {
	for id := range l.work {
		c, ok := l.Get(id)
		if !ok {
			continue
		}
		if !c.Probed {
			l.probe(&c)
		}
		t := l.tools()
		if t.FFmpeg == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		mid := c.Duration / 2
		if mid > 3 {
			mid = math.Min(mid, 3+c.Duration/4)
		}
		thumbErr := ffmpeg.Run(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
			"-ss", fmt.Sprintf("%.2f", mid), "-i", c.Path, "-frames:v", "1",
			"-vf", "scale=480:-2", "-q:v", "4", l.ThumbPath(id))
		frames := 10
		spriteErr := errors.New("skipped")
		if c.Duration > 0 {
			spriteErr = ffmpeg.Run(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
				"-skip_frame", "nokey", "-i", c.Path, "-an",
				"-vf", fmt.Sprintf("fps=%d/%.3f,scale=240:-2,tile=%dx1", frames, c.Duration, frames),
				"-frames:v", "1", "-q:v", "5", l.SpritePath(id))
		}
		cancel()
		l.mu.Lock()
		if cur, ok := l.clips[id]; ok {
			if c.Probed && !cur.Probed {
				cur.Duration, cur.Width, cur.Height, cur.FPS, cur.Codec, cur.Probed = c.Duration, c.Width, c.Height, c.FPS, c.Codec, true
				if len(cur.Tracks) == 0 {
					cur.Tracks = c.Tracks
				}
			}
			cur.Thumb = thumbErr == nil
			cur.Sprite = spriteErr == nil
			cur.Frames = frames
		}
		l.mu.Unlock()
		l.changed()
	}
}

type Patch struct {
	Title    *string   `json:"title"`
	Favorite *bool     `json:"favorite"`
	Tags     *[]string `json:"tags"`
	Game     *string   `json:"game"`
}

func (l *Library) Update(id string, p Patch) (Clip, error) {
	l.mu.Lock()
	c, ok := l.clips[id]
	if !ok {
		l.mu.Unlock()
		return Clip{}, errors.New("clip not found")
	}
	if p.Title != nil {
		if t := strings.TrimSpace(*p.Title); t != "" {
			c.Title = t
		}
	}
	if p.Favorite != nil {
		c.Favorite = *p.Favorite
	}
	if p.Tags != nil {
		c.Tags = *p.Tags
	}
	if p.Game != nil {
		c.Game = *p.Game
	}
	out := *c
	l.mu.Unlock()
	l.changed()
	return out, nil
}

func (l *Library) dropFiles(id string) {
	os.Remove(l.ThumbPath(id))
	os.Remove(l.SpritePath(id))
	for i := 0; i < 4; i++ {
		os.Remove(l.wavePath(id, i))
	}
}

// recycle bin, not delete
func (l *Library) Delete(ids []string) error {
	l.mu.Lock()
	var paths []string
	for _, id := range ids {
		if c, ok := l.clips[id]; ok {
			paths = append(paths, c.Path)
		}
	}
	l.mu.Unlock()
	var err error
	if len(paths) > 0 {
		err = winapi.Recycle(paths...)
	}
	l.mu.Lock()
	for _, id := range ids {
		c, ok := l.clips[id]
		if !ok {
			continue
		}
		if _, statErr := os.Stat(c.Path); statErr != nil {
			delete(l.clips, id)
			l.dropFiles(id)
		}
	}
	l.mu.Unlock()
	l.changed()
	return err
}

func (l *Library) Scan(clipsDir, exportsDir string) {
	var fresh []string
	l.mu.Lock()
	known := map[string]bool{}
	for _, c := range l.clips {
		known[strings.ToLower(c.Path)] = true
	}
	l.mu.Unlock()
	isVideo := func(name string) bool {
		ext := strings.ToLower(filepath.Ext(name))
		return ext == ".mp4" || ext == ".mov" || ext == ".mkv"
	}
	// two levels: per-game folders, but not all of C:\ if someone picks it
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if depth < 2 && !strings.HasPrefix(e.Name(), ".") {
					walk(p, depth+1)
				}
				continue
			}
			if isVideo(e.Name()) && !known[strings.ToLower(p)] {
				known[strings.ToLower(p)] = true
				fresh = append(fresh, p)
			}
		}
	}
	walk(clipsDir, 0)
	walk(exportsDir, 0)
	l.mu.Lock()
	removed := false
	for id, c := range l.clips {
		if _, err := os.Stat(c.Path); errors.Is(err, fs.ErrNotExist) {
			delete(l.clips, id)
			l.dropFiles(id)
			removed = true
		}
	}
	var retry []string
	for id, c := range l.clips {
		if !c.Thumb {
			if _, err := os.Stat(l.ThumbPath(id)); err != nil {
				retry = append(retry, id)
			} else {
				c.Thumb = true
			}
		}
	}
	for _, p := range fresh {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		c := &Clip{
			ID: newID(), Path: p,
			Title:     strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)),
			Game:      guessGame(p, clipsDir),
			CreatedAt: st.ModTime(), Size: st.Size(), Tags: []string{},
			Export: isUnder(p, exportsDir),
		}
		l.clips[c.ID] = c
		retry = append(retry, c.ID)
	}
	l.mu.Unlock()
	for _, id := range retry {
		l.enqueue(id)
	}
	if removed || len(fresh) > 0 {
		l.changed()
	}
}

func guessGame(path, clipsDir string) string {
	if parent := filepath.Dir(path); !strings.EqualFold(parent, clipsDir) && isUnder(path, clipsDir) && !strings.EqualFold(filepath.Base(parent), "Exports") {
		return filepath.Base(parent)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	f := strings.Fields(base)
	for i, w := range f {
		if len(w) == 10 && strings.Count(w, "-") == 2 && i > 0 {
			return strings.Join(f[:i], " ")
		}
	}
	return ""
}

func isUnder(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

type Usage struct {
	UsedBytes   int64 `json:"usedBytes"`
	ClipCount   int   `json:"clipCount"`
	FreeBytes   int64 `json:"freeBytes"`
	TotalBytes  int64 `json:"totalBytes"`
	BudgetBytes int64 `json:"budgetBytes"`
}

func (l *Library) Usage(clipsDir string, s config.Settings) Usage {
	l.mu.Lock()
	u := Usage{ClipCount: len(l.clips)}
	for _, c := range l.clips {
		u.UsedBytes += c.Size
	}
	l.mu.Unlock()
	os.MkdirAll(clipsDir, 0o755)
	if free, total, err := winapi.DiskFree(clipsDir); err == nil {
		u.FreeBytes, u.TotalBytes = int64(free), int64(total)
	}
	if s.StorageBudgetEnabled {
		u.BudgetBytes = int64(s.StorageBudgetGB) << 30
	}
	return u
}

// skip favorites
func (l *Library) Prune(budget int64) int {
	if budget <= 0 {
		return 0
	}
	l.mu.Lock()
	var total int64
	list := make([]*Clip, 0, len(l.clips))
	for _, c := range l.clips {
		total += c.Size
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	removed := 0
	for _, c := range list {
		if total <= budget {
			break
		}
		if c.Favorite {
			continue
		}
		if err := os.Remove(c.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		total -= c.Size
		delete(l.clips, c.ID)
		l.dropFiles(c.ID)
		removed++
	}
	l.mu.Unlock()
	if removed > 0 {
		l.changed()
	}
	return removed
}

// decoding audio per open is slow
func (l *Library) Waveform(id string, track, buckets int) ([]float32, error) {
	c, ok := l.Get(id)
	if !ok {
		return nil, errors.New("clip not found")
	}
	if buckets <= 0 || buckets > 4000 {
		buckets = 1200
	}
	cache := l.wavePath(id, track)
	if b, err := os.ReadFile(cache); err == nil && len(b) == buckets*4 {
		out := make([]float32, buckets)
		for i := range out {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
		}
		return out, nil
	}
	t := l.tools()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, err := ffmpeg.Output(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-i", c.Path,
		"-map", fmt.Sprintf("0:a:%d", track), "-ac", "1", "-ar", "4000", "-f", "s16le", "-")
	if err != nil {
		return nil, err
	}
	n := len(raw) / 2
	out := make([]float32, buckets)
	if n == 0 {
		return out, nil
	}
	var peak float32
	for i := 0; i < n; i++ {
		v := float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
		if v < 0 {
			v = -v
		}
		b := i * buckets / n
		if v > out[b] {
			out[b] = v
		}
		if v > peak {
			peak = v
		}
	}
	if peak > 0 {
		for i := range out {
			out[i] /= peak
		}
	}
	enc := make([]byte, buckets*4)
	for i, v := range out {
		binary.LittleEndian.PutUint32(enc[i*4:], math.Float32bits(v))
	}
	os.WriteFile(cache, enc, 0o644)
	return out, nil
}

func (l *Library) FindByPath(path string) (Clip, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.clips {
		if strings.EqualFold(c.Path, path) {
			return *c, true
		}
	}
	return Clip{}, false
}

func (l *Library) Edit(id string, fn func(c *Clip)) (Clip, bool) {
	l.mu.Lock()
	c, ok := l.clips[id]
	if ok {
		fn(c)
	}
	var out Clip
	if ok {
		out = *c
	}
	l.mu.Unlock()
	if ok {
		l.changed()
	}
	return out, ok
}

func (l *Library) ReplaceFile(id, newPath string) (Clip, bool) {
	l.mu.Lock()
	c, ok := l.clips[id]
	if !ok {
		l.mu.Unlock()
		return Clip{}, false
	}
	oldStem := strings.TrimSuffix(filepath.Base(c.Path), filepath.Ext(c.Path))
	if c.Title == oldStem {
		c.Title = strings.TrimSuffix(filepath.Base(newPath), filepath.Ext(newPath))
	}
	c.Path = newPath
	c.Probed, c.Thumb, c.Sprite = false, false, false
	if st, err := os.Stat(newPath); err == nil {
		c.Size = st.Size()
	}
	snapshot := *c
	l.mu.Unlock()
	l.probe(&snapshot)
	l.mu.Lock()
	if cur, ok := l.clips[id]; ok {
		cur.Duration, cur.Width, cur.Height, cur.FPS, cur.Codec, cur.Probed = snapshot.Duration, snapshot.Width, snapshot.Height, snapshot.FPS, snapshot.Codec, snapshot.Probed
		if snapshot.Size > 0 {
			cur.Size = snapshot.Size
		}
		cur.Tracks = snapshot.Tracks
		snapshot = *cur
	}
	l.mu.Unlock()
	l.dropFiles(id)
	l.changed()
	l.enqueue(id)
	return snapshot, true
}

func (l *Library) ShrinkCandidates(olderThan time.Duration) []Clip {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Clip
	for _, c := range l.clips {
		if !c.Shrunk && !c.Export && c.Probed && time.Since(c.CreatedAt) > olderThan && !strings.Contains(strings.ToLower(c.Codec), "av1") {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
