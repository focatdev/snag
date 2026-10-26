package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"snag/internal/winapi"
)

// set by the release workflow via -ldflags -X
var (
	appVersion = "0.1.0"
	updateRepo = ""
)

type UpdateState struct {
	Available bool   `json:"available"`
	Checking  bool   `json:"checking"`
	Latest    string `json:"latest"`
	Ready     bool   `json:"ready"`
	Error     string `json:"error"`
	CheckedAt int64  `json:"checkedAt"`
}

type WhatsNew struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

type updater struct {
	mu    sync.Mutex
	state UpdateState
	busy  bool
}

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Body       string    `json:"body"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

func newerVersion(tag, cur string) bool {
	parse := func(v string) ([3]int, string) {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		main, pre, _ := strings.Cut(v, "-")
		var n [3]int
		for i, p := range strings.SplitN(main, ".", 3) {
			n[i], _ = strconv.Atoi(p)
		}
		return n, pre
	}
	a, ap := parse(tag)
	b, bp := parse(cur)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return ap == "" && bp != ""
}

func (c *Core) updateState() UpdateState {
	c.upd.mu.Lock()
	defer c.upd.mu.Unlock()
	s := c.upd.state
	s.Available = updateRepo != ""
	return s
}

func (c *Core) setUpdate(fn func(*UpdateState)) {
	c.upd.mu.Lock()
	fn(&c.upd.state)
	c.upd.mu.Unlock()
	c.emit("update", c.updateState())
}

func cleanupOldExe() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(exe + ".new")
	}
}

func (c *Core) updateLoop() {
	select {
	case <-c.ctx.Done():
		return
	case <-time.After(20 * time.Second):
	}
	for {
		if c.store.Get().AutoUpdate {
			c.checkUpdate(false)
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(6 * time.Hour):
		}
	}
}

func (c *Core) checkUpdate(manual bool) error {
	if updateRepo == "" {
		return errors.New("updates come with release builds from GitHub")
	}
	c.upd.mu.Lock()
	if c.upd.busy || c.upd.state.Ready {
		c.upd.mu.Unlock()
		return nil
	}
	c.upd.busy = true
	c.upd.mu.Unlock()
	defer func() {
		c.upd.mu.Lock()
		c.upd.busy = false
		c.upd.mu.Unlock()
	}()
	c.setUpdate(func(s *UpdateState) { s.Checking, s.Error = true, "" })
	latest, err := c.fetchAndApply()
	c.setUpdate(func(s *UpdateState) {
		s.Checking, s.CheckedAt = false, time.Now().UnixMilli()
		if latest != "" {
			s.Latest = latest
		}
		if err != nil {
			s.Error = err.Error()
		}
	})
	if err != nil {
		slog.Warn("update", "err", err)
		if manual {
			return err
		}
	}
	return nil
}

func (c *Core) fetchAndApply() (string, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Minute}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "snag/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't reach GitHub: %w", err)
	}
	var rel ghRelease
	err = json.NewDecoder(resp.Body).Decode(&rel)
	resp.Body.Close()
	if resp.StatusCode == 404 {
		return "", errors.New("no releases published yet")
	}
	if err != nil || resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub answered %s", resp.Status)
	}
	if !newerVersion(rel.Tag, appVersion) {
		return rel.Tag, nil
	}

	var exeAsset, sums *ghAsset
	for i, a := range rel.Assets {
		switch strings.ToLower(a.Name) {
		case "snag.exe":
			exeAsset = &rel.Assets[i]
		case "checksums.txt":
			sums = &rel.Assets[i]
		}
	}
	if exeAsset == nil {
		return rel.Tag, errors.New("the release has no Snag.exe")
	}
	// asset digests only exist on newer releases
	want := strings.TrimPrefix(exeAsset.Digest, "sha256:")
	if want == "" && sums != nil {
		want = fetchSum(ctx, client, sums.URL, exeAsset.Name)
	}
	if want == "" {
		return rel.Tag, errors.New("the release has no checksum, not installing it")
	}

	exe, err := os.Executable()
	if err != nil {
		return rel.Tag, err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	tmp := exe + ".new"
	req, _ = http.NewRequestWithContext(ctx, "GET", exeAsset.URL, nil)
	req.Header.Set("User-Agent", "snag/"+appVersion)
	resp, err = client.Do(req)
	if err != nil {
		return rel.Tag, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return rel.Tag, fmt.Errorf("download answered %s", resp.Status)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return rel.Tag, fmt.Errorf("can't write next to Snag.exe: %w", err)
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return rel.Tag, err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want) {
		os.Remove(tmp)
		return rel.Tag, errors.New("the download didn't match its checksum, skipped it")
	}

	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return rel.Tag, err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe)
		return rel.Tag, err
	}
	b, _ := json.Marshal(WhatsNew{Version: strings.TrimPrefix(rel.Tag, "v"), Notes: rel.Body})
	os.WriteFile(filepath.Join(c.dataDir, "whatsnew.json"), b, 0o644)
	slog.Info("update ready", "from", appVersion, "to", rel.Tag)
	c.setUpdate(func(s *UpdateState) { s.Ready = true })
	c.toast("info", "Snag "+strings.TrimPrefix(rel.Tag, "v")+" is ready, restart to finish")
	return rel.Tag, nil
}

func fetchSum(ctx context.Context, client *http.Client, url, name string) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "snag/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.EqualFold(strings.TrimPrefix(f[1], "*"), name) {
			return f[0]
		}
	}
	return ""
}

func (c *Core) whatsNew() *WhatsNew {
	s := c.store.Get()
	if s.LastVersion == appVersion {
		return nil
	}
	first := s.LastVersion == ""
	s.LastVersion = appVersion
	c.store.Set(s)
	if first {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(c.dataDir, "whatsnew.json"))
	if err != nil {
		return nil
	}
	var w WhatsNew
	if json.Unmarshal(b, &w) != nil || w.Version != strings.TrimPrefix(appVersion, "v") {
		return nil
	}
	return &w
}

func (c *Core) restartForUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := winapi.RunAfterExit(`start "" "` + exe + `"`); err != nil {
		return err
	}
	go c.app.Quit()
	return nil
}
