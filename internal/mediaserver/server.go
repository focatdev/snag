package mediaserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type Resolver func(kind, id string) (string, bool)

// webview2 can't seek big files without range requests
type Server struct {
	Base  string
	token string
}

func Start(resolve Resolver) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	rand.Read(b)
	s := &Server{token: hex.EncodeToString(b)}
	s.Base = fmt.Sprintf("http://%s", ln.Addr().String())

	mux := http.NewServeMux()
	mux.HandleFunc("/m/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		kind, id := r.PathValue("kind"), r.PathValue("id")
		path, ok := resolve(kind, strings.TrimSuffix(id, ".jpg"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if kind == "thumb" || kind == "sprite" {
			w.Header().Set("Cache-Control", "max-age=3600")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, path)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	return s, nil
}

func (s *Server) Token() string { return s.token }
