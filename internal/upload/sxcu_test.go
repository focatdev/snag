package upload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	c := &evalCtx{body: `{"data":{"link":"https://i.example/a.mp4","files":[{"url":"u0"},{"url":"u1"}],"id":42}}`, filename: "clip.mp4"}
	cases := map[string]string{
		"{json:data.link}":                           "https://i.example/a.mp4",
		"{json:$.data.files[1].url}":                 "u1",
		"{json:data['id']}":                          "42",
		"https://x.io/{json:data.id}/raw":            "https://x.io/42/raw",
		`literal \{braces\} and \| pipe`:             "literal {braces} and | pipe",
		"{filename}":                                 "clip.mp4",
		"Basic {base64:user:pass}":                   "Basic dXNlcjpwYXNz",
		"{select:a.com|b.com}":                       "a.com",
		`{regex:"link":"([^"]+)"|1}`:                 "https://i.example/a.mp4",
		`{regex:"id":(?<num>\d+)|num}`:               "42",
		`{regex:{json:data.link}|https://([^/]+)|1}`: "i.example",
	}
	for in, want := range cases {
		got, err := expand(in, c)
		if err != nil || got != want {
			t.Errorf("expand(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	x := &evalCtx{body: `<files><file><url>first</url></file><file id="7"><url>second</url></file></files>`}
	for in, want := range map[string]string{"{xml:/files/file[2]/url}": "second", "{xml:/files/file[2]/@id}": "7", "{xml:/files/file/url}": "first"} {
		if got, _ := expand(in, x); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLegacyAndReject(t *testing.T) {
	u, err := ParseSXCU([]byte(`{"Name":"Old","RequestType":"POST","RequestURL":"https://old.example/up","FileFormName":"f","URL":"$json:url$","Arguments":{"n":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if u.URL != "{json:url}" || u.Body != "MultipartFormData" || u.Arguments["n"] != "5" {
		t.Fatalf("legacy parse: %+v", u)
	}
	if _, err := ParseSXCU([]byte(`{"RequestURL":"https://x.example","Body":"JSON"}`)); err == nil {
		t.Fatal("json body should be rejected")
	}
}

func TestSXCUUploads(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Roblox clip.mp4")
	os.WriteFile(p, []byte("video"), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" || r.URL.Query().Get("expiry") != "7d" {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"bad key"}`)
			return
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			f, h, err := r.FormFile("upload")
			if err != nil || r.FormValue("title") != "Roblox clip.mp4" {
				w.WriteHeader(400)
				return
			}
			b, _ := io.ReadAll(f)
			io.WriteString(w, `{"files":[{"url":"https://h.example/`+string(b)+`","name":"`+h.Filename+`","del":"https://h.example/d/1"}]}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Location", "https://h.example/bin/"+string(b))
		w.WriteHeader(201)
	}))
	defer srv.Close()

	multi := SXCU{Name: "Test", RequestMethod: "POST", RequestURL: srv.URL + "/up", Body: "MultipartFormData", FileFormName: "upload",
		Parameters: map[string]string{"expiry": "7d"}, Headers: map[string]string{"Authorization": "Bearer k"},
		Arguments: map[string]string{"title": "{filename}"}, URL: "{json:files[0].url}", DeletionURL: "{json:files[0].del}", ErrorMessage: "{json:error}"}
	res, err := multi.Upload(context.Background(), p, nil)
	if err != nil || res.URL != "https://h.example/video" || res.DeletionURL != "https://h.example/d/1" {
		t.Fatalf("multipart: %+v %v", res, err)
	}

	bin := multi
	bin.Body, bin.RequestMethod, bin.URL = "Binary", "PUT", "{header:location}"
	if res, err := bin.Upload(context.Background(), p, nil); err != nil || res.URL != "https://h.example/bin/video" {
		t.Fatalf("binary: %+v %v", res, err)
	}

	bad := multi
	bad.Headers = map[string]string{"Authorization": "Bearer wrong"}
	if _, err := bad.Upload(context.Background(), p, nil); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("error message not parsed: %v", err)
	}
}
