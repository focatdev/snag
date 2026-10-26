package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"snag/internal/config"
	"snag/internal/upload"
)

type UploaderInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Builtin    bool   `json:"builtin"`
	VideoReady bool   `json:"videoReady"`
}

const catboxID = "catbox"

func (c *Core) uploadersDir() string { return filepath.Join(c.dataDir, "uploaders") }

func (c *Core) listUploaders() []UploaderInfo {
	out := []UploaderInfo{{ID: catboxID, Name: "Catbox", Host: "catbox.moe", Builtin: true, VideoReady: true}}
	files, _ := filepath.Glob(filepath.Join(c.uploadersDir(), "*.sxcu"))
	var custom []UploaderInfo
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		u, err := upload.ParseSXCU(b)
		if err != nil {
			continue
		}
		custom = append(custom, UploaderInfo{ID: strings.TrimSuffix(filepath.Base(f), ".sxcu"), Name: u.Name, Host: hostOf(u.RequestURL), VideoReady: u.VideoReady()})
	}
	sort.Slice(custom, func(i, j int) bool { return strings.ToLower(custom[i].Name) < strings.ToLower(custom[j].Name) })
	return append(out, custom...)
}

func hostOf(raw string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?{"); i >= 0 {
		s = s[:i]
	}
	return s
}

// keep our own copy, people delete the original
func (c *Core) importUploader(data []byte) (UploaderInfo, error) {
	u, err := upload.ParseSXCU(data)
	if err != nil {
		return UploaderInfo{}, err
	}
	sum := sha1.Sum([]byte(u.Name + "\x00" + u.RequestURL))
	id := slug(u.Name) + "-" + hex.EncodeToString(sum[:3])
	if err := os.MkdirAll(c.uploadersDir(), 0o755); err != nil {
		return UploaderInfo{}, err
	}
	b, _ := json.MarshalIndent(u, "", "  ")
	if err := config.WriteFileAtomic(filepath.Join(c.uploadersDir(), id+".sxcu"), b); err != nil {
		return UploaderInfo{}, err
	}
	return UploaderInfo{ID: id, Name: u.Name, Host: hostOf(u.RequestURL), VideoReady: u.VideoReady()}, nil
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 32 {
		out = out[:32]
	}
	if out == "" {
		out = "uploader"
	}
	return out
}

func (c *Core) deleteUploader(id string) error {
	if id == catboxID || strings.ContainsAny(id, `/\.`) {
		return errors.New("that uploader can't be removed")
	}
	if err := os.Remove(filepath.Join(c.uploadersDir(), id+".sxcu")); err != nil {
		return err
	}
	if s := c.store.Get(); s.Uploader == id {
		s.Uploader = catboxID
		c.store.Set(s)
		c.emit("settings:changed", masked(s))
	}
	return nil
}

func (c *Core) loadUploader(id string) (upload.SXCU, error) {
	if strings.ContainsAny(id, `/\.`) {
		return upload.SXCU{}, errors.New("bad uploader id")
	}
	b, err := os.ReadFile(filepath.Join(c.uploadersDir(), id+".sxcu"))
	if err != nil {
		return upload.SXCU{}, errors.New("that custom uploader is gone, pick another in Settings")
	}
	return upload.ParseSXCU(b)
}
