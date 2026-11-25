package voice

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"snag/internal/progress"
)

// pinned: snag calls the c api with structs laid out for this exact version
const (
	kwsLibDLL  = "sherpa-onnx-c-api.dll"
	kwsLibURL  = "https://github.com/k2-fsa/sherpa-onnx/releases/download/v2.0.1/sherpa-onnx-v2.0.1-win-x64-shared-MT-Release-no-tts-lib.tar.bz2"
	kwsLibSHA  = "21da6a8b92d63ca02ad41dbfc120012343aaaec2e5e02b14819c6b4f38004509"
	kwsLibSize = 7655517
	kwsVerFile = "version.txt"
	kwsVersion = "sherpa-onnx v2.0.1, kws-zipformer-gigaspeech-3.3M-2024-01-01"

	// not the -mobile export, its encoder fails a reshape under the onnxruntime this release ships
	kwsModelURL  = "https://github.com/k2-fsa/sherpa-onnx/releases/download/kws-models/sherpa-onnx-kws-zipformer-gigaspeech-3.3M-2024-01-01.tar.bz2"
	kwsModelSHA  = "f170013b4716e41b62b9bfd809687c207cef798ef9bc6534d524e17af9b6561a"
	kwsModelSize = 17626723
)

var kwsDLLs = []string{"onnxruntime.dll", "onnxruntime_providers_shared.dll", kwsLibDLL}

type Model struct {
	Encoder, Decoder, Joiner, Tokens, Bpe string
}

func modelOf(dir string) Model {
	m := filepath.Join(dir, "model")
	return Model{
		Encoder: filepath.Join(m, "encoder.onnx"),
		Decoder: filepath.Join(m, "decoder.onnx"),
		Joiner:  filepath.Join(m, "joiner.onnx"),
		Tokens:  filepath.Join(m, "tokens.txt"),
		Bpe:     filepath.Join(m, "bpe.model"),
	}
}

func Find(dir string) (Model, error) {
	m := modelOf(dir)
	b, _ := os.ReadFile(filepath.Join(dir, kwsVerFile))
	if strings.TrimSpace(string(b)) != kwsVersion {
		return m, errors.New("download the keyword spotter in Settings, Voice")
	}
	for _, p := range append([]string{m.Encoder, m.Decoder, m.Joiner, m.Tokens, m.Bpe}, filepath.Join(dir, kwsLibDLL)) {
		if !exists(p) {
			return m, errors.New("download the keyword spotter in Settings, Voice")
		}
	}
	return m, nil
}

func Download(ctx context.Context, dir string, onProgress func(done, total int64)) error {
	if err := os.MkdirAll(filepath.Join(dir, "model"), 0o755); err != nil {
		return err
	}
	client := &http.Client{}
	pw := &progress.Writer{Total: kwsLibSize + kwsModelSize, Fn: onProgress}
	get := func(url, sha string) (string, error) {
		part := filepath.Join(dir, path.Base(url)+".part")
		sum, err := fetch(ctx, client, url, part, pw)
		if err != nil {
			os.Remove(part)
			return "", err
		}
		if !strings.EqualFold(sum, sha) {
			os.Remove(part)
			return "", errors.New("checksum mismatch, download was corrupted")
		}
		return part, nil
	}
	lib, err := get(kwsLibURL, kwsLibSHA)
	if err != nil {
		return err
	}
	defer os.Remove(lib)
	model, err := get(kwsModelURL, kwsModelSHA)
	if err != nil {
		return err
	}
	defer os.Remove(model)
	return installKWS(dir, lib, model)
}

func installKWS(dir, lib, model string) error {
	if err := os.MkdirAll(filepath.Join(dir, "model"), 0o755); err != nil {
		return err
	}
	err := untar(lib, func(name string) string {
		base := path.Base(name)
		for _, d := range kwsDLLs {
			if strings.EqualFold(base, d) && path.Base(path.Dir(name)) == "lib" {
				return filepath.Join(dir, d)
			}
		}
		return ""
	})
	if err != nil {
		return err
	}
	m := modelOf(dir)
	// the int8 files are the light ones
	pick := map[string]string{}
	err = untar(model, func(name string) string {
		base := path.Base(name)
		switch base {
		case "tokens.txt":
			return m.Tokens
		case "bpe.model":
			return m.Bpe
		}
		for _, part := range []string{"encoder", "decoder", "joiner"} {
			if !strings.HasPrefix(base, part+"-") || !strings.HasSuffix(base, ".onnx") {
				continue
			}
			int8 := strings.HasSuffix(base, ".int8.onnx")
			if prev, ok := pick[part]; ok && (strings.HasSuffix(prev, ".int8.onnx") || !int8) {
				return ""
			}
			pick[part] = base
			return filepath.Join(dir, "model", part+".onnx")
		}
		return ""
	})
	if err != nil {
		return err
	}
	for _, d := range kwsDLLs {
		if !exists(filepath.Join(dir, d)) {
			return fmt.Errorf("%s missing from the download", d)
		}
	}
	for _, p := range []string{m.Encoder, m.Decoder, m.Joiner, m.Tokens, m.Bpe} {
		if !exists(p) {
			return fmt.Errorf("%s missing from the download", filepath.Base(p))
		}
	}
	return os.WriteFile(filepath.Join(dir, kwsVerFile), []byte(kwsVersion+"\n"), 0o644)
}

// dest returns where an entry goes, empty skips it
func untar(file string, dest func(name string) string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(bzip2.NewReader(f))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		final := dest(h.Name)
		if final == "" {
			continue
		}
		if err := writeFile(tr, final); err != nil {
			return err
		}
	}
}

func writeFile(r io.Reader, final string) error {
	tmp := final + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, r)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	// a loaded dll can't be overwritten but can be renamed
	if exists(final) {
		old := final + ".old"
		os.Remove(old)
		if os.Rename(final, old) == nil {
			os.Remove(old)
		}
	}
	return os.Rename(tmp, final)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// fetch writes the body to dst and returns its sha256 hex digest
func fetch(ctx context.Context, client *http.Client, url, dst string, pw *progress.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "snag")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download returned %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h, pw), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
