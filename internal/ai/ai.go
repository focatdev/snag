package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Provider struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Model      string `json:"model"`
	Transcribe bool   `json:"transcribe"`
	KeyURL     string `json:"keyUrl"`
}

// defaults as of oct 2026
var Providers = []Provider{
	{ID: "groq", Label: "Groq", Model: "qwen/qwen3.8-27b", Transcribe: true, KeyURL: "https://console.groq.com/keys"},
	{ID: "gemini", Label: "Gemini", Model: "gemini-3.5-flash-lite", Transcribe: true, KeyURL: "https://aistudio.google.com/apikey"},
	{ID: "openai", Label: "OpenAI", Model: "gpt-6-luna", Transcribe: true, KeyURL: "https://platform.openai.com/api-keys"},
	{ID: "anthropic", Label: "Claude", Model: "claude-haiku-5-5", Transcribe: false, KeyURL: "https://console.anthropic.com/settings/keys"},
}

func Lookup(id string) (Provider, bool) {
	for _, p := range Providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

type Client struct {
	Provider string
	Key      string
	Model    string
	http     *http.Client
}

func New(provider, key, model string) (*Client, error) {
	p, ok := Lookup(provider)
	if !ok {
		return nil, errors.New("pick an AI provider in Settings")
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("add your " + p.Label + " key in Settings")
	}
	if strings.TrimSpace(model) == "" {
		model = p.Model
	}
	return &Client{Provider: provider, Key: strings.TrimSpace(key), Model: model, http: &http.Client{Timeout: 90 * time.Second}}, nil
}

type Description struct {
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
}

func prompt(game string) string {
	g := game
	if g == "" || g == "Desktop" {
		g = "an unknown game"
	}
	return "These frames come from one short gameplay clip of " + g + ", in order. " +
		"Write a title that says what happens, like a player would name the clip: at most 6 words, " +
		"no quotes, no game name, no emojis. Then give 2 to 5 short lowercase tags. " +
		`Reply with JSON only: {"title": "...", "tags": ["..."]}`
}

var jsonBlock = regexp.MustCompile(`(?s)\{.*\}`)

func parseDescription(text string) (Description, error) {
	var d Description
	m := jsonBlock.FindString(text)
	if m == "" {
		return d, fmt.Errorf("unexpected reply: %s", clip(text, 160))
	}
	if err := json.Unmarshal([]byte(m), &d); err != nil {
		return d, fmt.Errorf("unexpected reply: %s", clip(text, 160))
	}
	d.Title = strings.Trim(strings.TrimSpace(d.Title), `"'`)
	if d.Title == "" {
		return d, errors.New("the model returned an empty title")
	}
	if len(d.Tags) > 5 {
		d.Tags = d.Tags[:5]
	}
	for i := range d.Tags {
		d.Tags[i] = strings.ToLower(strings.TrimSpace(d.Tags[i]))
	}
	return d, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func (c *Client) post(ctx context.Context, url string, headers map[string]string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(out, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = clip(string(out), 200)
		}
		return nil, fmt.Errorf("%s: %s", resp.Status, msg)
	}
	return out, nil
}

func (c *Client) Describe(ctx context.Context, frames [][]byte, game string) (Description, error) {
	text := prompt(game)
	switch c.Provider {
	case "openai", "groq":
		content := []any{map[string]any{"type": "text", "text": text}}
		for _, f := range frames {
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(f)}})
		}
		base := "https://api.openai.com/v1"
		if c.Provider == "groq" {
			base = "https://api.groq.com/openai/v1"
		}
		out, err := c.post(ctx, base+"/chat/completions", map[string]string{"Authorization": "Bearer " + c.Key}, map[string]any{
			"model":    c.Model,
			"messages": []any{map[string]any{"role": "user", "content": content}},
		})
		if err != nil {
			return Description{}, err
		}
		var r struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(out, &r); err != nil || len(r.Choices) == 0 {
			return Description{}, errors.New("empty reply from " + c.Provider)
		}
		return parseDescription(r.Choices[0].Message.Content)
	case "gemini":
		parts := []any{map[string]any{"text": text}}
		for _, f := range frames {
			parts = append(parts, map[string]any{"inline_data": map[string]string{"mime_type": "image/jpeg", "data": base64.StdEncoding.EncodeToString(f)}})
		}
		return c.gemini(ctx, parts)
	case "anthropic":
		var content []any
		for _, f := range frames {
			content = append(content, map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": "image/jpeg", "data": base64.StdEncoding.EncodeToString(f)}})
		}
		content = append(content, map[string]any{"type": "text", "text": text})
		out, err := c.post(ctx, "https://api.anthropic.com/v1/messages", map[string]string{"x-api-key": c.Key, "anthropic-version": "2023-06-01"}, map[string]any{
			"model":      c.Model,
			"max_tokens": 300,
			"messages":   []any{map[string]any{"role": "user", "content": content}},
		})
		if err != nil {
			return Description{}, err
		}
		var r struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(out, &r); err != nil || len(r.Content) == 0 {
			return Description{}, errors.New("empty reply from Claude")
		}
		return parseDescription(r.Content[0].Text)
	}
	return Description{}, errors.New("unknown provider")
}

func (c *Client) gemini(ctx context.Context, parts []any) (Description, error) {
	text, err := c.geminiText(ctx, parts, true)
	if err != nil {
		return Description{}, err
	}
	return parseDescription(text)
}

func (c *Client) geminiText(ctx context.Context, parts []any, asJSON bool) (string, error) {
	body := map[string]any{"contents": []any{map[string]any{"parts": parts}}}
	if asJSON {
		body["generationConfig"] = map[string]any{"responseMimeType": "application/json"}
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + c.Model + ":generateContent"
	out, err := c.post(ctx, url, map[string]string{"x-goog-api-key": c.Key}, body)
	if err != nil {
		return "", err
	}
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(out, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("empty reply from Gemini")
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}

func (c *Client) Transcribe(ctx context.Context, audio []byte) (string, error) {
	switch c.Provider {
	case "openai", "groq":
		url, model := "https://api.openai.com/v1/audio/transcriptions", "gpt-transcribe"
		if c.Provider == "groq" {
			url, model = "https://api.groq.com/openai/v1/audio/transcriptions", "whisper-large-v3-turbo"
		}
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		w.WriteField("model", model)
		w.WriteField("response_format", "json")
		fw, _ := w.CreateFormFile("file", "voice.m4a")
		fw.Write(audio)
		w.Close()
		req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+c.Key)
		out, err := c.do(req)
		if err != nil {
			return "", err
		}
		var r struct {
			Text string `json:"text"`
		}
		json.Unmarshal(out, &r)
		return strings.TrimSpace(r.Text), nil
	case "gemini":
		parts := []any{
			map[string]any{"text": "Transcribe the speech in this audio. Reply with the transcript only, nothing else. Reply with an empty string if nobody speaks."},
			map[string]any{"inline_data": map[string]string{"mime_type": "audio/mp4", "data": base64.StdEncoding.EncodeToString(audio)}},
		}
		text, err := c.geminiText(ctx, parts, false)
		return strings.TrimSpace(text), err
	}
	return "", errors.New("voice search needs Groq, Gemini or OpenAI")
}
