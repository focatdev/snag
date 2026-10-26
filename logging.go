package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type recentLog struct {
	mu    sync.Mutex
	lines []string
}

func (r *recentLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		r.lines = append(r.lines, l)
	}
	if len(r.lines) > 300 {
		r.lines = r.lines[len(r.lines)-300:]
	}
	return len(p), nil
}

func (r *recentLog) tail(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) < n {
		n = len(r.lines)
	}
	return append([]string(nil), r.lines[len(r.lines)-n:]...)
}

var logTail = &recentLog{}

func logPath(dir string) string { return filepath.Join(dir, "snag.log") }

// keep one old log around for crashes
func setupLog(dir string) {
	p := logPath(dir)
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		os.Rename(p, filepath.Join(dir, "snag.old.log"))
	}
	var out io.Writer = logTail
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		out = io.MultiWriter(f, logTail)
	}
	h := slog.NewTextHandler(out, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format("01-02 15:04:05"))
			}
			return a
		},
	})
	slog.SetDefault(slog.New(h))
	slog.Info("start", "version", appVersion, "pid", os.Getpid(), "at", time.Now().Format(time.RFC3339))
}
