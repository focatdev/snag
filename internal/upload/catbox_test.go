package upload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCatboxForm(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Roblox 2026-10-25.mp4")
	os.WriteFile(p, []byte("fake video bytes"), 0o644)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f, h, err := r.FormFile("fileToUpload")
		if err != nil || r.FormValue("reqtype") != "fileupload" || r.FormValue("userhash") != "abc" {
			http.Error(w, "bad form", 400)
			return
		}
		b, _ := io.ReadAll(f)
		if string(b) != "fake video bytes" || h.Filename != "clip.mp4" {
			http.Error(w, "bad file", 400)
			return
		}
		io.WriteString(w, "https://files.catbox.moe/x8f2kd.mp4")
	}))
	defer srv.Close()
	catboxAPI = srv.URL
	var last int64
	url, err := Catbox(context.Background(), p, "abc", func(done, total int64) { last = done })
	if err != nil || url != "https://files.catbox.moe/x8f2kd.mp4" {
		t.Fatalf("got %q %v", url, err)
	}
	if last == 0 {
		t.Error("no progress reported")
	}
}
