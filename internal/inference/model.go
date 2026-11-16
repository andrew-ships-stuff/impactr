package inference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Model struct {
	Name, Filename, URL, SHA256, License, LicenseURL string
	Size                                             int64
}

var DefaultModel = Model{Name: "qwen2.5-0.5b-instruct-q4_0", Filename: "qwen2.5-0.5b-instruct-q4_0.gguf", URL: "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_0.gguf", SHA256: "7671c0c304e6ce5a7fc577bcb12aba01e2c155cc2efd29b2213c95b18edaf6ed", License: "Apache-2.0", LicenseURL: "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/blob/main/LICENSE", Size: 428730208}

type ModelManager struct {
	CacheDir string
	Client   *http.Client
}

func DefaultCacheDir() string {
	if p := os.Getenv("IMPACTR_MODEL_CACHE"); p != "" {
		return p
	}
	d, e := os.UserCacheDir()
	if e != nil {
		return filepath.Join(os.TempDir(), "impactr-models")
	}
	return filepath.Join(d, "impactr", "models")
}
func (m ModelManager) Path(model Model) string {
	d := m.CacheDir
	if d == "" {
		d = DefaultCacheDir()
	}
	return filepath.Join(d, model.Filename)
}
func (m ModelManager) Verify(model Model) (string, error) {
	p := m.Path(model)
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != model.SHA256 {
		return "", fmt.Errorf("model checksum mismatch: got %s", got)
	}
	return p, nil
}
func (m ModelManager) Install(ctx context.Context, model Model, progress func(int64, int64)) (string, error) {
	if p, e := m.Verify(model); e == nil {
		return p, nil
	}
	p := m.Path(model)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return "", e
	}
	tmp := p + ".partial"
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, model.URL, nil)
	if e != nil {
		return "", e
	}
	c := m.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Minute}
	}
	resp, e := c.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("model download returned %s", resp.Status)
	}
	f, e := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	var n int64
	buf := make([]byte, 1<<20)
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			if _, e = f.Write(buf[:nr]); e != nil {
				f.Close()
				return "", e
			}
			h.Write(buf[:nr])
			n += int64(nr)
			if progress != nil {
				progress(n, model.Size)
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			f.Close()
			return "", er
		}
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != model.SHA256 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("model checksum mismatch: got %s", got)
	}
	if e = os.Rename(tmp, p); e != nil {
		return "", e
	}
	return p, nil
}
