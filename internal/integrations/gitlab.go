package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type GitLab struct {
	BaseURL, Project, Token string
	Client                  *http.Client
}

func (g GitLab) request(ctx context.Context, method, path string, body io.Reader, ctype string) (*http.Response, error) {
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://gitlab.com"
	}
	u := base + "/api/v4/projects/" + url.PathEscape(g.Project) + path
	req, e := http.NewRequestWithContext(ctx, method, u, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("PRIVATE-TOKEN", g.Token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	c := g.Client
	if c == nil {
		c = http.DefaultClient
	}
	return c.Do(req)
}
func (g GitLab) Comment(ctx context.Context, mr int, markdown string, images []string) error {
	if g.Token == "" {
		return fmt.Errorf("GITLAB_TOKEN is required")
	}
	for _, imagePath := range images {
		f, e := os.Open(imagePath)
		if e != nil {
			return e
		}
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		p, _ := mw.CreateFormFile("file", filepath.Base(imagePath))
		_, e = io.Copy(p, f)
		f.Close()
		mw.Close()
		if e != nil {
			return e
		}
		resp, e := g.request(ctx, http.MethodPost, "/uploads", &b, mw.FormDataContentType())
		if e != nil {
			return e
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("GitLab upload: %s", resp.Status)
		}
		var up struct {
			Markdown string `json:"markdown"`
		}
		if e = json.Unmarshal(raw, &up); e != nil {
			return e
		}
		base := filepath.Base(imagePath)
		lines := strings.Split(markdown, "\n")
		for i, ln := range lines {
			if strings.Contains(ln, "]("+base+")") {
				lines[i] = up.Markdown
			}
		}
		markdown = strings.Join(lines, "\n")
	}
	path := "/merge_requests/" + strconv.Itoa(mr) + "/notes"
	resp, e := g.request(ctx, http.MethodGet, path+"?per_page=100", nil, "")
	if e != nil {
		return e
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	var notes []struct {
		ID   int    `json:"id"`
		Body string `json:"body"`
	}
	_ = json.Unmarshal(raw, &notes)
	payload, _ := json.Marshal(map[string]string{"body": markdown})
	for _, n := range notes {
		if strings.Contains(n.Body, "<!-- impactr-comment:v1 -->") {
			resp, e = g.request(ctx, http.MethodPut, path+"/"+strconv.Itoa(n.ID), bytes.NewReader(payload), "application/json")
			return check(resp, e)
		}
	}
	resp, e = g.request(ctx, http.MethodPost, path, bytes.NewReader(payload), "application/json")
	return check(resp, e)
}
func check(resp *http.Response, e error) error {
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("integration API returned %s", resp.Status)
	}
	return nil
}
