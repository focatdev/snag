package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"snag/internal/ai"
	"snag/internal/config"
	"snag/internal/ffmpeg"
	"snag/internal/gamedetect"
	"snag/internal/jobs"
	"snag/internal/library"
	"snag/internal/upload"
	"snag/internal/winapi"
)

func uniqueFile(p string) string {
	if _, err := os.Stat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(c); err != nil {
			return c
		}
	}
}

// rename fails across drives
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	out, err := os.Create(to)
	if err != nil {
		in.Close()
		return err
	}
	_, err = io.Copy(out, in)
	in.Close()
	out.Close()
	if err != nil {
		os.Remove(to)
		return err
	}
	return os.Remove(from)
}

func (c *Core) jobDone(j jobs.Job) {
	s := c.store.Get()
	switch j.Kind {
	case "export":
		orig, ok := c.lib.Get(j.ClipID)
		if j.Replace && ok {
			dest := strings.TrimSuffix(orig.Path, filepath.Ext(orig.Path)) + ".mp4"
			if err := winapi.Recycle(orig.Path); err != nil {
				c.toast("error", "Could not replace the original: "+err.Error())
				return
			}
			if err := moveFile(j.Output, dest); err != nil {
				c.toast("error", "Trimmed copy kept in Exports: "+err.Error())
				c.lib.Add(library.NewClip{Path: j.Output, Game: orig.Game, Export: true})
				return
			}
			c.lib.ReplaceFile(orig.ID, dest)
			c.lib.Edit(orig.ID, func(cl *library.Clip) { cl.Markers = nil })
			c.toast("success", fmt.Sprintf("Original replaced, %s smaller", humanBytes(orig.Size-j.Size)))
			return
		}
		c.lib.Add(library.NewClip{Path: j.Output, Game: orig.Game, Export: true})
		if s.CopyExports {
			if err := winapi.CopyFiles(j.Output); err == nil {
				c.toast("success", "Export copied, ready to paste")
			}
		}
	case "merge":
		first, _ := c.lib.Get(j.ClipID)
		c.lib.Add(library.NewClip{Path: j.Output, Game: first.Game, Export: true})
		if s.CopyExports {
			winapi.CopyFiles(j.Output)
		}
		c.toast("success", "Montage saved")
	case "shrink":
		orig, ok := c.lib.Get(j.ClipID)
		st, err := os.Stat(j.Output)
		if !ok || err != nil {
			os.Remove(j.Output)
			return
		}
		// not worth a re-encode
		if float64(st.Size()) > float64(orig.Size)*0.9 {
			os.Remove(j.Output)
			c.lib.Edit(orig.ID, func(cl *library.Clip) { cl.Shrunk = true })
			return
		}
		if err := os.Rename(j.Output, orig.Path); err != nil {
			os.Remove(j.Output)
			c.toast("error", "Could not shrink "+orig.Title+": "+err.Error())
			return
		}
		c.lib.ReplaceFile(orig.ID, orig.Path)
		c.lib.Edit(orig.ID, func(cl *library.Clip) {
			cl.Shrunk = true
			if cl.OrigSize == 0 {
				cl.OrigSize = orig.Size
			}
		})
	}
}

func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}

func (c *Core) shrinkSource(id string) (jobs.Source, bool) {
	cl, ok := c.lib.Get(id)
	if !ok {
		return jobs.Source{}, false
	}
	return jobs.Source{ID: cl.ID, Path: cl.Path, Title: cl.Title, Size: cl.Size, Duration: cl.Duration, Width: cl.Width, Height: cl.Height, FPS: cl.FPS, Tracks: len(cl.Tracks)}, true
}

// never runs while a game has focus
func (c *Core) shrinkLoop() {
	t := time.NewTicker(3 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-t.C:
		}
		s := c.store.Get()
		if s.AutoShrinkDays == 0 || c.jobs.Busy() || gamedetect.Current().IsGame || c.Tools().FFmpeg == "" {
			continue
		}
		list := c.lib.ShrinkCandidates(time.Duration(s.AutoShrinkDays) * 24 * time.Hour)
		if len(list) == 0 {
			continue
		}
		if src, ok := c.shrinkSource(list[0].ID); ok {
			c.jobs.SubmitShrink(src)
		}
	}
}

func (c *Core) screenshot() {
	s := c.store.Get()
	t := c.Tools()
	if t.FFmpeg == "" {
		c.toast("error", "FFmpeg is not installed")
		return
	}
	game := gamedetect.Recent(2 * time.Minute).Name
	dir := filepath.Join(s.ClipsDir, "Screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.toast("error", err.Error())
		return
	}
	out := uniqueFile(filepath.Join(dir, config.RenderName(s.FileTemplate, game, 0, time.Now())+".png"))
	ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
	defer cancel()
	err := ffmpeg.Run(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("ddagrab=output_idx=%d:framerate=30", s.Monitor),
		"-frames:v", "1", "-vf", "hwdownload,format=bgra", out)
	if err != nil {
		m := winapi.MonitorAt(s.Monitor)
		err = ffmpeg.Run(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
			"-f", "gdigrab", "-offset_x", fmt.Sprint(m.Left), "-offset_y", fmt.Sprint(m.Top),
			"-video_size", fmt.Sprintf("%dx%d", m.Width, m.Height), "-i", "desktop", "-frames:v", "1", out)
	}
	if err != nil {
		playCue(s, "error")
		c.toast("error", "Screenshot failed: "+err.Error())
		return
	}
	playCue(s, "shutter")
	sub := "In Snag\\Screenshots"
	if s.CopyScreenshots && winapi.CopyFiles(out) == nil {
		sub = "Copied, ready to paste"
	}
	c.overlay(winapi.ToastSuccess, "Screenshot saved", sub)
	c.toast("success", "Screenshot saved")
}

func (c *Core) aiClient() (*ai.Client, error) {
	s := c.store.Get()
	key, err := winapi.Unprotect(s.AIKey)
	if err != nil {
		return nil, err
	}
	return ai.New(s.AIProvider, key, s.AIModel)
}

func (c *Core) queueAI(id string) {
	s := c.store.Get()
	if s.AIProvider == "" || (!s.AIAutoTitle && !s.AITranscribe) {
		return
	}
	select {
	case c.aiQueue <- id:
	default:
	}
}

// serial on purpose, free api tiers rate limit bursts
func (c *Core) aiWorker() {
	warned := false
	for {
		select {
		case <-c.ctx.Done():
			return
		case id := <-c.aiQueue:
			s := c.store.Get()
			var err error
			if s.AIAutoTitle {
				_, err = c.describeClip(id)
			}
			if err == nil && s.AITranscribe {
				_, err = c.transcribeClip(id)
			}
			if err != nil && !warned {
				warned = true
				c.toast("error", "AI: "+err.Error())
			}
		}
	}
}

func (c *Core) aiTemp() string {
	d := filepath.Join(c.dataDir, "tmp", "ai")
	os.MkdirAll(d, 0o755)
	return d
}

func (c *Core) describeClip(id string) (library.Clip, error) {
	cl, ok := c.lib.Get(id)
	if !ok {
		return cl, errors.New("clip not found")
	}
	client, err := c.aiClient()
	if err != nil {
		return cl, err
	}
	t := c.Tools()
	dir, err := os.MkdirTemp(c.aiTemp(), "frames")
	if err != nil {
		return cl, err
	}
	defer os.RemoveAll(dir)
	dur := cl.Duration
	if dur <= 0 {
		dur = 30
	}
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Minute)
	defer cancel()
	// 3 images fits every provider's per-request cap
	if err := ffmpeg.Run(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-skip_frame", "nokey", "-i", cl.Path,
		"-vf", fmt.Sprintf("fps=3/%.2f,scale=640:-2", dur), "-frames:v", "3", "-q:v", "4", filepath.Join(dir, "f%d.jpg")); err != nil {
		return cl, err
	}
	var frames [][]byte
	for i := 1; i <= 3; i++ {
		if b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("f%d.jpg", i))); err == nil {
			frames = append(frames, b)
		}
	}
	if len(frames) == 0 {
		return cl, errors.New("could not read frames from the clip")
	}
	d, err := client.Describe(ctx, frames, cl.Game)
	if err != nil {
		return cl, err
	}
	out, _ := c.lib.Edit(id, func(x *library.Clip) {
		x.Title, x.AITitled = d.Title, true
		seen := map[string]bool{}
		for _, t := range x.Tags {
			seen[t] = true
		}
		for _, t := range d.Tags {
			if t != "" && !seen[t] {
				x.Tags = append(x.Tags, t)
				seen[t] = true
			}
		}
	})
	return out, nil
}

func (c *Core) transcribeClip(id string) (library.Clip, error) {
	cl, ok := c.lib.Get(id)
	if !ok {
		return cl, errors.New("clip not found")
	}
	if len(cl.Tracks) == 0 {
		return cl, errors.New("this clip has no audio")
	}
	client, err := c.aiClient()
	if err != nil {
		return cl, err
	}
	track := 0
	for i, name := range cl.Tracks {
		if name == "Mic" || name == "Voice" {
			track = i
			break
		}
	}
	dir, err := os.MkdirTemp(c.aiTemp(), "voice")
	if err != nil {
		return cl, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "voice.m4a")
	ctx, cancel := context.WithTimeout(c.ctx, 3*time.Minute)
	defer cancel()
	if err := ffmpeg.Run(ctx, c.Tools().FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", cl.Path,
		"-map", fmt.Sprintf("0:a:%d", track), "-t", "600", "-ac", "1", "-ar", "16000", "-c:a", "aac", "-b:a", "32k", file); err != nil {
		return cl, err
	}
	audio, err := os.ReadFile(file)
	if err != nil {
		return cl, err
	}
	text, err := client.Transcribe(ctx, audio)
	if err != nil {
		return cl, err
	}
	out, _ := c.lib.Edit(id, func(x *library.Clip) { x.Transcript = text })
	return out, nil
}

func (c *Core) uploadClip(id, title string) (jobs.Job, error) {
	s := c.store.Get()
	cl, ok := c.lib.Get(id)
	if !ok {
		return jobs.Job{}, errors.New("clip not found")
	}
	hostName := "catbox"
	var send func(ctx context.Context, progress func(done, total int64)) (upload.Result, error)
	if s.Uploader == "" || s.Uploader == catboxID {
		if cl.Size > upload.CatboxMax {
			return jobs.Job{}, errors.New("over catbox's 200 MB limit, export a smaller copy first")
		}
		if strings.EqualFold(filepath.Ext(cl.Path), ".gif") && cl.Size > 20<<20 {
			return jobs.Job{}, errors.New("catbox takes GIFs up to 20 MB, export it as MP4 instead")
		}
		send = func(ctx context.Context, progress func(done, total int64)) (upload.Result, error) {
			u, err := upload.Catbox(ctx, cl.Path, s.CatboxUserhash, progress)
			return upload.Result{URL: u}, err
		}
	} else {
		u, err := c.loadUploader(s.Uploader)
		if err != nil {
			return jobs.Job{}, err
		}
		hostName = u.Name
		send = func(ctx context.Context, progress func(done, total int64)) (upload.Result, error) {
			return u.Upload(ctx, cl.Path, progress)
		}
	}
	if title == "" {
		title = "Uploading"
	}
	sub := "Uploading to " + hostName
	return c.jobs.SubmitTask("upload", id, "Upload to "+hostName, func(ctx context.Context, progress func(float64)) (string, error) {
		res, err := send(ctx, func(done, total int64) {
			p := float64(done) / float64(max(total, 1))
			progress(p)
			c.overlayProgress(title, sub, p)
		})
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.overlay(winapi.ToastError, "Upload failed", err.Error())
				c.toast("error", "Upload failed: "+err.Error())
			}
			return "", err
		}
		winapi.CopyText(res.URL)
		c.lib.Edit(id, func(x *library.Clip) { x.Link, x.DeleteLink = res.URL, res.DeletionURL })
		c.overlay(winapi.ToastSuccess, "Link copied", strings.TrimPrefix(strings.TrimPrefix(res.URL, "https://"), "http://"))
		c.toast("success", "Link copied, paste it anywhere")
		// never delete favorites
		if s.CatboxDeleteAfter && !cl.Favorite {
			c.lib.Delete([]string{id})
		}
		return res.URL, nil
	}), nil
}

// older builds could leave a half-written frame at the end
func (c *Core) repairClip(id string) (library.Clip, error) {
	cl, ok := c.lib.Get(id)
	if !ok {
		return library.Clip{}, errors.New("clip not found")
	}
	if cl.Duration < 1 {
		return library.Clip{}, errors.New("clip is too short to repair")
	}
	t := c.Tools()
	if t.FFmpeg == "" {
		return library.Clip{}, errors.New("FFmpeg is not installed")
	}
	tmp := filepath.Join(filepath.Dir(cl.Path), ".snag-repair-"+id+filepath.Ext(cl.Path))
	ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
	defer cancel()
	err := ffmpeg.Run(ctx, t.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", cl.Path, "-map", "0", "-c", "copy",
		"-t", fmt.Sprintf("%.3f", cl.Duration-0.15), "-avoid_negative_ts", "make_zero", "-movflags", "+faststart", tmp)
	if err != nil {
		os.Remove(tmp)
		return library.Clip{}, err
	}
	// webview can hold the file briefly after src is cleared
	for i := 0; ; i++ {
		if err = os.Rename(tmp, cl.Path); err == nil || i == 20 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		os.Remove(tmp)
		return library.Clip{}, err
	}
	fixed, ok := c.lib.ReplaceFile(id, cl.Path)
	if !ok {
		return library.Clip{}, errors.New("clip vanished while repairing")
	}
	return fixed, nil
}
