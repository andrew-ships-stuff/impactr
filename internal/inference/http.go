package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPProvider struct {
	ProviderName, Endpoint, Model, APIKey string
	Client                                *http.Client
}

func (h HTTPProvider) Name() string { return h.ProviderName }
func (h HTTPProvider) Explain(ctx context.Context, r Request) (Explanation, error) {
	endpoint := strings.TrimRight(h.Endpoint, "/")
	if h.ProviderName == "ollama" {
		endpoint += "/api/chat"
	} else {
		endpoint += "/v1/chat/completions"
	}
	u, e := url.Parse(endpoint)
	if e != nil || !(u.Scheme == "http" || u.Scheme == "https") {
		return Explanation{}, fmt.Errorf("invalid inference endpoint")
	}
	var body any
	if h.ProviderName == "ollama" {
		body = map[string]any{"model": h.Model, "stream": false, "messages": []map[string]string{{"role": "user", "content": Prompt(r)}}, "options": map[string]any{"temperature": 0, "num_predict": r.MaxTokens}}
	} else {
		body = map[string]any{"model": h.Model, "temperature": 0, "max_tokens": r.MaxTokens, "messages": []map[string]string{{"role": "user", "content": Prompt(r)}}}
	}
	b, _ := json.Marshal(body)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if e != nil {
		return Explanation{}, e
	}
	req.Header.Set("Content-Type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, e := client.Do(req)
	if e != nil {
		return Explanation{}, e
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return Explanation{}, e
	}
	if resp.StatusCode/100 != 2 {
		return Explanation{}, fmt.Errorf("inference endpoint returned %s", resp.Status)
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
	if e != nil {
		return Explanation{}, e
	}
	out, e := Parse(text, r)
	out.Provider = h.ProviderName
	out.Model = h.Model
	return out, e
}
