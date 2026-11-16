package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/progress"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPProvider struct {
	ProviderName, Endpoint, Model, APIKey string
	Client                                *http.Client
	Report                                func(progress.Event)
}

func (h HTTPProvider) Name() string { return h.ProviderName }

func (h HTTPProvider) chat(ctx context.Context, prompt string, maxTokens int) (string, error) {
	endpoint := strings.TrimRight(h.Endpoint, "/")
	if h.ProviderName == "ollama" {
		endpoint += "/api/chat"
	} else {
		endpoint += "/v1/chat/completions"
	}
	u, e := url.Parse(endpoint)
	if e != nil || !(u.Scheme == "http" || u.Scheme == "https") {
		return "", fmt.Errorf("invalid inference endpoint")
	}
	var body any
	if h.ProviderName == "ollama" {
		body = map[string]any{"model": h.Model, "stream": false, "messages": []map[string]string{{"role": "user", "content": prompt}}, "options": map[string]any{"temperature": 0, "num_predict": maxTokens}}
	} else {
		body = map[string]any{"model": h.Model, "temperature": 0, "max_tokens": maxTokens, "messages": []map[string]string{{"role": "user", "content": prompt}}}
	}
	b, _ := json.Marshal(body)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	e = nil
	if h.Report != nil {
		h.Report(progress.Event{Phase: "inference", Detail: fmt.Sprintf("%s via %s", h.Model, h.ProviderName)})
	}
	reqStart := time.Now()
	resp, e := client.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if h.Report != nil {
		h.Report(progress.Event{Phase: "inference", Detail: fmt.Sprintf("%s · %d bytes", resp.Status, len(raw)), Done: true, Err: e, Elapsed: time.Since(reqStart)})
	}
	if e != nil {
		return "", e
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("inference endpoint returned %s", resp.Status)
	}
	var text string
	if h.ProviderName == "ollama" {
		var x struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		e = json.Unmarshal(raw, &x)
		text = x.Message.Content
	} else {
		var x struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		e = json.Unmarshal(raw, &x)
		if len(x.Choices) > 0 {
			text = x.Choices[0].Message.Content
		}
	}
	return text, e
}

func (h HTTPProvider) Explain(ctx context.Context, r Request) (Explanation, error) {
	text, e := h.chat(ctx, Prompt(r), r.MaxTokens)
	if e != nil {
		return Explanation{}, e
	}
	out, e := Parse(text, r)
	out.Provider = h.ProviderName
	out.Model = h.Model
	return out, e
}

func (h HTTPProvider) Brief(ctx context.Context, r BriefRequest) (Brief, error) {
	tokens := r.MaxTokens
	if tokens <= 0 {
		tokens = 256
	}
	text, e := h.chat(ctx, BriefPrompt(r), tokens)
	if e != nil {
		return Brief{}, e
	}
	out, e := ParseBrief(text, r)
	out.Provider = h.ProviderName
	out.Model = h.Model
	return out, e
}
