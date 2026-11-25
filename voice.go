package main

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"snag/internal/config"
	"snag/internal/recorder"
	"snag/internal/voice"
)

type voiceState struct {
	mu    sync.Mutex
	stop  context.CancelFunc
	key   string
	err   string
	dir   string
	ready bool
	dl    downloadState
}

type voiceStatus struct {
	Ready    bool          `json:"ready"`
	Dir      string        `json:"dir"`
	Error    string        `json:"error"`
	Download downloadState `json:"download"`
}

func (c *Core) refreshVoiceReady() {
	dir := c.store.Get().VoiceInstallDir(c.dataDir)
	_, err := voice.Find(dir)
	c.voice.mu.Lock()
	c.voice.dir = dir
	c.voice.ready = err == nil
	c.voice.mu.Unlock()
}

func (c *Core) voiceStatus() voiceStatus {
	c.voice.mu.Lock()
	defer c.voice.mu.Unlock()
	return voiceStatus{
		Ready:    c.voice.ready,
		Dir:      c.voice.dir,
		Error:    c.voice.err,
		Download: c.voice.dl,
	}
}

func (c *Core) downloadVoice() error {
	err := c.runDownload(&c.voice.mu, &c.voice.dl, func(progress func(done, total int64)) error {
		return voice.Download(c.ctx, c.store.Get().VoiceInstallDir(c.dataDir), progress)
	})
	if err != nil {
		return err
	}
	c.refreshVoiceReady()
	c.toast("success", "Keyword spotter is ready")
	c.syncVoice()
	c.pushStatus()
	return nil
}

func voiceKeyOf(s config.Settings, dir string, ready, buffering bool) string {
	if !s.VoiceTrigger || !ready || !buffering {
		return ""
	}
	return strings.Join([]string{s.VoicePhrase, s.MicDeviceID, dir, strconv.Itoa(s.VoiceSensitivity)}, "\x00")
}

func (c *Core) syncVoice() {
	s := c.store.Get()
	buffering := c.rec.State() == recorder.Buffering
	c.voice.mu.Lock()
	defer c.voice.mu.Unlock()
	k := voiceKeyOf(s, c.voice.dir, c.voice.ready, buffering)
	if k == c.voice.key {
		return
	}
	if c.voice.stop != nil {
		c.voice.stop()
		c.voice.stop = nil
	}
	c.voice.key = k
	c.voice.err = ""
	if k == "" {
		return
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.voice.stop = cancel
	dir := c.voice.dir
	go c.runVoice(ctx, k, dir, s)
}

func (c *Core) runVoice(ctx context.Context, key, dir string, s config.Settings) {
	heard := func(phrase string) {
		slog.Info("voice trigger", "heard", phrase)
		go c.saveFull()
	}

	err := voice.Listen(ctx, voice.Options{
		Mic:         s.MicDeviceID,
		Dir:         dir,
		Phrase:      s.VoicePhrase,
		Sensitivity: float64(s.VoiceSensitivity) / 100,
	}, heard)
	if err != nil && ctx.Err() == nil {
		c.voice.mu.Lock()
		if c.voice.key == key {
			c.voice.err = err.Error()
		}
		c.voice.mu.Unlock()
		c.toast("error", "Voice trigger: "+err.Error())
		c.pushStatus()
	}
}
