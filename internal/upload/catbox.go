package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CatboxMax = 200 << 20

var catboxAPI = "https://catbox.moe/user/api.php"

var ErrTooBig = errors.New("catbox takes files up to 200 MB")

type counter struct {
	r     io.Reader
	done  int64
	total int64
	fn    func(done, total int64)
	last  time.Time
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	if c.fn != nil && (time.Since(c.last) > 150*time.Millisecond || err == io.EOF) {
		c.last = time.Now()
		c.fn(c.done, c.total)
	}
	return n, err
}

func Catbox(ctx context.Context, path, userhash string, progress func(done, total int64)) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if st.Size() > CatboxMax {
		return "", ErrTooBig
	}

	// stream from disk with a known length instead of buffering the file
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	mw.WriteField("reqtype", "fileupload")
	if h := strings.TrimSpace(userhash); h != "" {
		mw.WriteField("userhash", h)
	}
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="fileToUpload"; filename=%q`, safeName(filepath.Base(path))))
	hdr.Set("Content-Type", contentType(path))
	if _, err := mw.CreatePart(hdr); err != nil {
		return "", err
	}
	tail := "\r\n--" + mw.Boundary() + "--\r\n"

	body := &counter{r: io.MultiReader(bytes.NewReader(head.Bytes()), f, strings.NewReader(tail)), total: int64(head.Len()) + st.Size() + int64(len(tail)), fn: progress}
	req, err := http.NewRequestWithContext(ctx, "POST", catboxAPI, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = body.total
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", "snag")

	resp, err := (&http.Client{Timeout: 20 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(out))
	// catbox errors are plain text, sometimes with a 200
	if resp.StatusCode/100 != 2 || !strings.HasPrefix(text, "https://") {
		if text == "" {
			text = resp.Status
		}
		if len(text) > 160 {
			text = text[:160]
		}
		return "", errors.New("catbox: " + text)
	}
	return text, nil
}

// only the extension survives
func safeName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		ext = ".mp4"
	}
	return "clip" + ext
}

func contentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gif":
		return "image/gif"
	case ".png":
		return "image/png"
	case ".webm":
		return "video/webm"
	}
	return "video/mp4"
}
