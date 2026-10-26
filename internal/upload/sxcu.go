package upload

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SXCU struct {
	Version         string            `json:"Version,omitempty"`
	Name            string            `json:"Name,omitempty"`
	DestinationType string            `json:"DestinationType,omitempty"`
	RequestMethod   string            `json:"RequestMethod,omitempty"`
	RequestURL      string            `json:"RequestURL"`
	Parameters      map[string]string `json:"Parameters,omitempty"`
	Headers         map[string]string `json:"Headers,omitempty"`
	Body            string            `json:"Body,omitempty"`
	Arguments       map[string]string `json:"Arguments,omitempty"`
	FileFormName    string            `json:"FileFormName,omitempty"`
	Data            string            `json:"Data,omitempty"`
	URL             string            `json:"URL,omitempty"`
	ThumbnailURL    string            `json:"ThumbnailURL,omitempty"`
	DeletionURL     string            `json:"DeletionURL,omitempty"`
	ErrorMessage    string            `json:"ErrorMessage,omitempty"`
}

type Result struct {
	URL         string `json:"url"`
	DeletionURL string `json:"deletionUrl,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}

// old files use $json:...$ tokens and numeric argument values
func ParseSXCU(b []byte) (SXCU, error) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return SXCU{}, fmt.Errorf("not a valid .sxcu file: %w", err)
	}
	var u SXCU
	str := func(k string) string {
		v, ok := raw[k]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
		var n json.Number
		if json.Unmarshal(v, &n) == nil {
			return n.String()
		}
		return string(v)
	}
	dict := func(k string) map[string]string {
		v, ok := raw[k]
		if !ok {
			return nil
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) != nil {
			return nil
		}
		out := map[string]string{}
		for key, val := range m {
			var s string
			if json.Unmarshal(val, &s) == nil {
				out[key] = s
			} else {
				out[key] = strings.Trim(string(val), `"`)
			}
		}
		return out
	}
	u.Version, u.Name, u.DestinationType = str("Version"), str("Name"), str("DestinationType")
	u.RequestMethod, u.RequestURL, u.Body = strings.ToUpper(str("RequestMethod")), str("RequestURL"), str("Body")
	u.Parameters, u.Headers, u.Arguments = dict("Parameters"), dict("Headers"), dict("Arguments")
	u.FileFormName, u.Data = str("FileFormName"), str("Data")
	u.URL, u.ThumbnailURL, u.DeletionURL, u.ErrorMessage = str("URL"), str("ThumbnailURL"), str("DeletionURL"), str("ErrorMessage")
	// pre-13 files
	if u.RequestMethod == "" {
		u.RequestMethod = strings.ToUpper(str("RequestType"))
	}
	if u.RequestMethod == "" {
		u.RequestMethod = "POST"
	}
	if u.Body == "" {
		if u.FileFormName != "" {
			u.Body = "MultipartFormData"
		} else {
			u.Body = "None"
		}
	}
	for _, p := range []*string{&u.RequestURL, &u.URL, &u.ThumbnailURL, &u.DeletionURL, &u.ErrorMessage} {
		*p = legacy(*p)
	}
	if strings.TrimSpace(u.RequestURL) == "" {
		return SXCU{}, errors.New("this uploader has no RequestURL")
	}
	if u.Name == "" {
		if pu, err := url.Parse(u.RequestURL); err == nil && pu.Host != "" {
			u.Name = pu.Host
		} else {
			u.Name = "Custom uploader"
		}
	}
	switch u.Body {
	case "MultipartFormData", "Binary":
	default:
		return SXCU{}, fmt.Errorf("%s sends files as %q, Snag can upload with multipart or binary bodies only", u.Name, u.Body)
	}
	if u.Body == "MultipartFormData" && u.FileFormName == "" {
		return SXCU{}, fmt.Errorf("%s has no FileFormName", u.Name)
	}
	return u, nil
}

var legacyTok = regexp.MustCompile(`\$(json|xml|regex|response|responseurl|header|filename|random|base64)(:[^$]*)?\$`)

func legacy(s string) string {
	return legacyTok.ReplaceAllStringFunc(s, func(m string) string {
		return "{" + strings.Trim(m, "$") + "}"
	})
}

func (u SXCU) VideoReady() bool {
	d := u.DestinationType
	return d == "" || strings.Contains(d, "FileUploader")
}

type evalCtx struct {
	body     string
	resp     *http.Response
	finalURL string
	filename string
	input    string
}

// {name:arg1|arg2}, \ escapes, nesting allowed
func expand(s string, c *evalCtx) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\\':
			if i+1 < len(s) {
				i++
				out.WriteByte(s[i])
			}
		case '{':
			end := matchBrace(s, i)
			if end < 0 {
				out.WriteString(s[i:])
				return out.String(), nil
			}
			v, err := call(s[i+1:end], c)
			if err != nil {
				return "", err
			}
			out.WriteString(v)
			i = end
		default:
			out.WriteByte(ch)
		}
	}
	return out.String(), nil
}

func matchBrace(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitArgs(s string) []string {
	var parts []string
	depth, last := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
		case '|':
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	return append(parts, s[last:])
}

func call(inner string, c *evalCtx) (string, error) {
	name, rawArgs, hasArgs := strings.Cut(inner, ":")
	name = strings.ToLower(strings.TrimSpace(name))
	var args []string
	if hasArgs {
		parts := splitArgs(rawArgs)
		// regex keeps its backslashes
		patIdx := -1
		if name == "regex" {
			patIdx = 0
			if len(parts) == 3 {
				patIdx = 1
			}
		}
		for i, a := range parts {
			if i == patIdx {
				args = append(args, unescapeSyntax(a))
				continue
			}
			v, err := expand(a, c)
			if err != nil {
				return "", err
			}
			args = append(args, v)
		}
	}
	switch name {
	case "response":
		return c.body, nil
	case "responseurl":
		return c.finalURL, nil
	case "header":
		if c.resp == nil || len(args) == 0 {
			return "", nil
		}
		return c.resp.Header.Get(args[0]), nil
	case "filename":
		return c.filename, nil
	case "input":
		return c.input, nil
	case "random", "select":
		// no prompts mid-upload, take the first option
		if len(args) == 0 {
			return "", nil
		}
		if name == "select" {
			return args[0], nil
		}
		return args[rand.IntN(len(args))], nil
	case "inputbox":
		if len(args) > 1 {
			return args[1], nil
		}
		return "", nil
	case "base64":
		return base64.StdEncoding.EncodeToString([]byte(strings.Join(args, "|"))), nil
	case "json":
		src, path := c.body, ""
		switch len(args) {
		case 1:
			path = args[0]
		case 2:
			src, path = args[0], args[1]
		default:
			return "", errors.New("json needs a path")
		}
		return jsonPath(src, path)
	case "xml":
		src, path := c.body, ""
		switch len(args) {
		case 1:
			path = args[0]
		case 2:
			src, path = args[0], args[1]
		default:
			return "", errors.New("xml needs a path")
		}
		return xmlPath(src, path)
	case "regex":
		src, pat, group := c.body, "", ""
		switch len(args) {
		case 1:
			pat = args[0]
		case 2:
			pat, group = args[0], args[1]
		case 3:
			src, pat, group = args[0], args[1], args[2]
		default:
			return "", errors.New("regex needs a pattern")
		}
		return regexGroup(src, pat, group)
	case "outputbox":
		return "", nil
	}
	return "", fmt.Errorf("unknown sxcu function %q", name)
}

func unescapeSyntax(s string) string {
	r := strings.NewReplacer(`\{`, "{", `\}`, "}", `\|`, "|", `\\`, `\`)
	return r.Replace(s)
}

func regexGroup(src, pat, group string) (string, error) {
	// .net (?<name>) vs go (?P<name>)
	pat = strings.ReplaceAll(pat, "(?<", "(?P<")
	pat = strings.ReplaceAll(pat, "(?P<=", "(?<=")
	pat = strings.ReplaceAll(pat, "(?P<!", "(?<!")
	re, err := regexp.Compile(pat)
	if err != nil {
		return "", fmt.Errorf("bad regex in uploader: %w", err)
	}
	m := re.FindStringSubmatch(src)
	if m == nil {
		return "", nil
	}
	if group == "" {
		return m[0], nil
	}
	if n, err := strconv.Atoi(group); err == nil {
		if n < len(m) {
			return m[n], nil
		}
		return "", nil
	}
	if i := re.SubexpIndex(group); i >= 0 {
		return m[i], nil
	}
	return "", nil
}

var pathTok = regexp.MustCompile(`\[(\d+)\]|\['([^']*)'\]|\["([^"]*)"\]|([^.\[\]]+)`)

// enough jsonpath for real sxcu files: a.b, a[0].b, $.a, ['key']
func jsonPath(src, path string) (string, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", errors.New("response is not json")
	}
	path = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(path), "$"), ".")
	for _, m := range pathTok.FindAllStringSubmatch(path, -1) {
		switch {
		case m[1] != "":
			arr, ok := v.([]any)
			n, _ := strconv.Atoi(m[1])
			if !ok || n >= len(arr) {
				return "", nil
			}
			v = arr[n]
		default:
			key := m[2] + m[3] + m[4]
			obj, ok := v.(map[string]any)
			if !ok {
				return "", nil
			}
			v = obj[key]
		}
	}
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	default:
		b, _ := json.Marshal(t)
		return string(b), nil
	}
}

type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []xmlNode  `xml:",any"`
}

var xstep = regexp.MustCompile(`^([^\[@]+)(?:\[(\d+)\])?$`)

// absolute paths and a trailing @attr, nothing fancier
func xmlPath(src, path string) (string, error) {
	var root xmlNode
	if err := xml.Unmarshal([]byte(src), &root); err != nil {
		return "", errors.New("response is not xml")
	}
	steps := strings.Split(strings.Trim(path, "/"), "/")
	if len(steps) == 0 {
		return "", nil
	}
	if steps[0] != root.XMLName.Local {
		return "", nil
	}
	cur := root
	for _, st := range steps[1:] {
		if strings.HasPrefix(st, "@") {
			for _, a := range cur.Attrs {
				if a.Name.Local == st[1:] {
					return a.Value, nil
				}
			}
			return "", nil
		}
		m := xstep.FindStringSubmatch(st)
		if m == nil {
			return "", nil
		}
		want := 1
		if m[2] != "" {
			want, _ = strconv.Atoi(m[2])
		}
		found := false
		for _, ch := range cur.Children {
			if ch.XMLName.Local == m[1] {
				want--
				if want == 0 {
					cur, found = ch, true
					break
				}
			}
		}
		if !found {
			return "", nil
		}
	}
	return strings.TrimSpace(cur.Text), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (u SXCU) Upload(ctx context.Context, path string, progress func(done, total int64)) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Result{}, err
	}
	pre := &evalCtx{filename: filepath.Base(path)}

	target, err := expand(u.RequestURL, pre)
	if err != nil {
		return Result{}, err
	}
	if len(u.Parameters) > 0 {
		pu, err := url.Parse(target)
		if err != nil {
			return Result{}, fmt.Errorf("bad RequestURL: %w", err)
		}
		q := pu.Query()
		for _, k := range sortedKeys(u.Parameters) {
			v, err := expand(u.Parameters[k], pre)
			if err != nil {
				return Result{}, err
			}
			q.Set(k, v)
		}
		pu.RawQuery = q.Encode()
		target = pu.String()
	}

	var body io.Reader
	var length int64
	contentType := ""
	switch u.Body {
	case "Binary":
		body, length, contentType = f, st.Size(), contentTypeFor(path)
	default:
		var head bytes.Buffer
		mw := multipart.NewWriter(&head)
		for _, k := range sortedKeys(u.Arguments) {
			v, err := expand(u.Arguments[k], pre)
			if err != nil {
				return Result{}, err
			}
			mw.WriteField(k, v)
		}
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, u.FileFormName, filepath.Base(path)))
		hdr.Set("Content-Type", contentTypeFor(path))
		if _, err := mw.CreatePart(hdr); err != nil {
			return Result{}, err
		}
		tail := "\r\n--" + mw.Boundary() + "--\r\n"
		body = io.MultiReader(bytes.NewReader(head.Bytes()), f, strings.NewReader(tail))
		length = int64(head.Len()) + st.Size() + int64(len(tail))
		contentType = mw.FormDataContentType()
	}
	cnt := &counter{r: body, total: length, fn: progress}
	req, err := http.NewRequestWithContext(ctx, u.RequestMethod, target, cnt)
	if err != nil {
		return Result{}, err
	}
	req.ContentLength = length
	req.Header.Set("User-Agent", "snag")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, k := range sortedKeys(u.Headers) {
		v, err := expand(u.Headers[k], pre)
		if err != nil {
			return Result{}, err
		}
		req.Header.Set(k, v)
	}

	resp, err := (&http.Client{Timeout: 20 * time.Minute}).Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	post := &evalCtx{body: strings.TrimSpace(string(raw)), resp: resp, finalURL: resp.Request.URL.String(), filename: pre.filename}

	if resp.StatusCode/100 != 2 {
		msg := ""
		if u.ErrorMessage != "" {
			msg, _ = expand(u.ErrorMessage, post)
		}
		if msg == "" {
			msg = post.body
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		if msg == "" {
			msg = resp.Status
		}
		return Result{}, fmt.Errorf("%s: %s", u.Name, msg)
	}
	var res Result
	if u.URL == "" {
		res.URL = post.body
	} else if res.URL, err = expand(u.URL, post); err != nil {
		return Result{}, err
	}
	res.URL = strings.TrimSpace(res.URL)
	if res.URL == "" {
		return Result{}, fmt.Errorf("%s answered without a link", u.Name)
	}
	if u.DeletionURL != "" {
		res.DeletionURL, _ = expand(u.DeletionURL, post)
	}
	if u.ThumbnailURL != "" {
		res.Thumbnail, _ = expand(u.ThumbnailURL, post)
	}
	return res, nil
}

func contentTypeFor(path string) string { return contentType(path) }
