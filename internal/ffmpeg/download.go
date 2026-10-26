package ffmpeg

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const releaseAPI = "https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/tags/latest"

var stableAsset = regexp.MustCompile(`^ffmpeg-n(\d+)\.(\d+)-latest-win64-gpl-\d+\.\d+\.zip$`)

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// "n8.1-3-gabc" -> 801, master -> 0
func StableVersion(v string) int {
	m := stableVer.FindStringSubmatch(v)
	if m == nil {
		return 0
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	return a*100 + b
}

var stableVer = regexp.MustCompile(`^n?(\d+)\.(\d+)`)

func assetVersion(name string) int {
	m := stableAsset.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	return a*100 + b
}

// master snapshots break encoders now and then
func pickAsset(assets []asset, below int) (asset, *asset, error) {
	var stable []asset
	var master, sums *asset
	for i, a := range assets {
		switch {
		case stableAsset.MatchString(a.Name):
			if below == 0 || assetVersion(a.Name) < below {
				stable = append(stable, a)
			}
		case a.Name == "ffmpeg-master-latest-win64-gpl.zip":
			master = &assets[i]
		case a.Name == "checksums.sha256":
			sums = &assets[i]
		}
	}
	if len(stable) > 0 {
		sort.Slice(stable, func(i, j int) bool {
			mi, mj := stableAsset.FindStringSubmatch(stable[i].Name), stableAsset.FindStringSubmatch(stable[j].Name)
			ai, _ := strconv.Atoi(mi[1])
			aj, _ := strconv.Atoi(mj[1])
			if ai != aj {
				return ai > aj
			}
			bi, _ := strconv.Atoi(mi[2])
			bj, _ := strconv.Atoi(mj[2])
			return bi > bj
		})
		return stable[0], sums, nil
	}
	if master != nil && below == 0 {
		return *master, sums, nil
	}
	if below > 0 {
		return asset{}, nil, errors.New("no older FFmpeg build is available")
	}
	return asset{}, nil, errors.New("no windows build found in release")
}

type progressWriter struct {
	done, total int64
	fn          func(done, total int64)
	last        int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.fn != nil && (p.done-p.last > 512<<10 || p.done == p.total) {
		p.last = p.done
		p.fn(p.done, p.total)
	}
	return len(b), nil
}

func Download(ctx context.Context, installDir string, below int, progress func(done, total int64)) (Tools, error) {
	client := &http.Client{}
	req, _ := http.NewRequestWithContext(ctx, "GET", releaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "snag")
	resp, err := client.Do(req)
	if err != nil {
		return Tools{}, err
	}
	var rel struct {
		Assets []asset `json:"assets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&rel)
	resp.Body.Close()
	if err != nil {
		return Tools{}, fmt.Errorf("reading release info: %w", err)
	}
	if resp.StatusCode != 200 {
		return Tools{}, fmt.Errorf("github returned %s", resp.Status)
	}
	a, sums, err := pickAsset(rel.Assets, below)
	if err != nil {
		return Tools{}, err
	}

	want := ""
	if sums != nil {
		want, _ = fetchChecksum(ctx, client, sums.URL, a.Name)
	}

	// keep the zip off C:, it's often nearly full
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return Tools{}, err
	}
	zipPath := filepath.Join(installDir, a.Name+".part")
	defer os.Remove(zipPath)

	req, _ = http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	req.Header.Set("User-Agent", "snag")
	resp, err = client.Do(req)
	if err != nil {
		return Tools{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Tools{}, fmt.Errorf("download returned %s", resp.Status)
	}
	f, err := os.Create(zipPath)
	if err != nil {
		return Tools{}, err
	}
	h := sha256.New()
	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}
	pw := &progressWriter{total: total, fn: progress}
	_, err = io.Copy(io.MultiWriter(f, h, pw), resp.Body)
	f.Close()
	if err != nil {
		return Tools{}, err
	}
	if want != "" && !strings.EqualFold(want, hex.EncodeToString(h.Sum(nil))) {
		return Tools{}, errors.New("checksum mismatch, download was corrupted")
	}

	if err := extract(zipPath, installDir, "ffmpeg.exe", "ffprobe.exe"); err != nil {
		return Tools{}, err
	}
	return Locate("", installDir)
}

func fetchChecksum(ctx context.Context, c *http.Client, url, name string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "snag")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return f[0], nil
		}
	}
	return "", errors.New("checksum not listed")
}

func extract(zipPath, dest string, names ...string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		base := path.Base(f.Name)
		if !contains(names, base) || !strings.Contains(f.Name, "/bin/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		tmp := filepath.Join(dest, base+".part")
		out, err := os.Create(tmp)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		final := filepath.Join(dest, base)
		// can't overwrite a running exe, renaming is fine
		if _, err := os.Stat(final); err == nil {
			old := final + ".old"
			os.Remove(old)
			if os.Rename(final, old) == nil {
				os.Remove(old)
			}
		}
		if err := os.Rename(tmp, final); err != nil {
			return err
		}
		found[base] = true
	}
	for _, n := range names {
		if !found[n] {
			return fmt.Errorf("%s missing from archive", n)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func Move(from, to string) error {
	if from == to {
		return nil
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	for _, n := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		src := filepath.Join(from, n)
		in, err := os.Open(src)
		if err != nil {
			continue
		}
		tmp := filepath.Join(to, n+".part")
		out, err := os.Create(tmp)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, filepath.Join(to, n)); err != nil {
			return err
		}
		os.Remove(src)
	}
	return nil
}

func NVENCTooOld(ctx context.Context, bin string) bool {
	log, err := testEncode(ctx, bin, "h264_nvenc", "error")
	return err != nil && strings.HasPrefix(Explain(log), "NVIDIA driver too old")
}

// 9.0 needs driver 610+, 8.1 works with 570+
func DownloadFitting(ctx context.Context, installDir string, below int, nvidia bool, progress func(done, total int64)) (Tools, error) {
	var t Tools
	var err error
	for step := 0; step < 3; step++ {
		nt, derr := Download(ctx, installDir, below, progress)
		if derr != nil {
			// nothing older, keep what we have
			if step > 0 {
				return t, nil
			}
			return nt, derr
		}
		t = nt
		if !nvidia || !NVENCTooOld(ctx, t.FFmpeg) {
			return t, nil
		}
		next := StableVersion(t.Version)
		if next == 0 {
			next = 1 << 20
		}
		if next == below {
			return t, nil
		}
		below = next
	}
	return t, err
}
