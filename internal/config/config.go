package config

import (
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type Config struct {
	Version   int       `yaml:"version"`
	Git       Git       `yaml:"git"`
	Analysis  Analysis  `yaml:"analysis"`
	Inference Inference `yaml:"inference"`
	Render    Render    `yaml:"render"`
	GitHub    Forge     `yaml:"github"`
	GitLab    Forge     `yaml:"gitlab"`
	Hooks     Hooks     `yaml:"hooks"`
}
type Git struct {
	Base             string `yaml:"base"`
	Comparison       string `yaml:"comparison"`
	IncludeUntracked bool   `yaml:"include_untracked"`
}
type Analysis struct {
	Languages        []string `yaml:"languages"`
	Exclude          []string `yaml:"exclude"`
	MaxDepth         int      `yaml:"max_depth"`
	MaxNodes         int      `yaml:"max_nodes"`
	Concurrency      int      `yaml:"concurrency"`
	IncludeTests     bool     `yaml:"include_tests"`
	IncludeGenerated bool     `yaml:"include_generated"`
}
type Inference struct {
	Provider        string `yaml:"provider"`
	Model           string `yaml:"model"`
	Endpoint        string `yaml:"endpoint"`
	Threads         int    `yaml:"threads"`
	MaxMemoryMB     int    `yaml:"max_memory_mb"`
	TimeoutSeconds  int    `yaml:"timeout_seconds"`
	MaxOutputTokens int    `yaml:"max_output_tokens"`
	ContextLength   int    `yaml:"context_length"`
	SendSource      bool   `yaml:"send_source"`
}
type Render struct {
	Views    []string `yaml:"views"`
	Formats  []string `yaml:"formats"`
	Theme    string   `yaml:"theme"`
	MaxNodes int      `yaml:"max_nodes"`
}
type Forge struct {
	Enabled        bool `yaml:"enabled"`
	Comment        bool `yaml:"comment"`
	UpdateExisting bool `yaml:"update_existing"`
}
type Hooks struct {
	Default        string `yaml:"default"`
	Blocking       bool   `yaml:"blocking"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

func Default() Config {
	return Config{Version: 1, Git: Git{Base: "auto", Comparison: "merge-base"}, Analysis: Analysis{Languages: []string{"go", "typescript", "python"}, Exclude: []string{"vendor/**", "node_modules/**", "**/*.generated.go"}, MaxDepth: 4, MaxNodes: 250, Concurrency: min(runtime.GOMAXPROCS(0), 16)}, Inference: Inference{Provider: "embedded", Model: "auto", Threads: max(1, runtime.GOMAXPROCS(0)/2), MaxMemoryMB: 2048, TimeoutSeconds: 120, MaxOutputTokens: 256, ContextLength: 4096}, Render: Render{Views: []string{"architecture", "diff", "impact"}, Formats: []string{"svg", "png", "markdown", "json"}, Theme: "auto", MaxNodes: 100}, Hooks: Hooks{Default: "pre-push", TimeoutSeconds: 30}, GitHub: Forge{Enabled: true, Comment: true, UpdateExisting: true}, GitLab: Forge{Enabled: true, Comment: true, UpdateExisting: true}}
}
func Load(root string) (Config, error) {
	c := Default()
	paths := []string{}
	if h, e := os.UserConfigDir(); e == nil {
		paths = append(paths, filepath.Join(h, "impactr", "config.yaml"))
	}
	paths = append(paths, filepath.Join(root, ".impactr.yaml"))
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return c, e
		}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if e = dec.Decode(&c); e != nil {
			return c, fmt.Errorf("%s: %w", p, e)
		}
	}
	applyEnv(&c)
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported configuration version %d", c.Version)
	}
	if c.Analysis.MaxDepth < 1 || c.Analysis.MaxDepth > 32 {
		return errors.New("analysis.max_depth must be between 1 and 32")
	}
	if c.Analysis.MaxNodes < 1 || c.Analysis.MaxNodes > 100000 {
		return errors.New("analysis.max_nodes must be between 1 and 100000")
	}
	if c.Inference.TimeoutSeconds < 1 {
		return errors.New("inference.timeout_seconds must be positive")
	}
	valid := map[string]bool{"embedded": true, "none": true, "ollama": true, "llama-server": true, "openai-compatible": true}
	if !valid[c.Inference.Provider] {
		return fmt.Errorf("unknown inference provider %q", c.Inference.Provider)
	}
	return nil
}
func applyEnv(c *Config) {
	if v := os.Getenv("IMPACTR_PROVIDER"); v != "" {
		c.Inference.Provider = v
	}
	if v := os.Getenv("IMPACTR_MODEL"); v != "" {
		c.Inference.Model = v
	}
	if v := os.Getenv("IMPACTR_ENDPOINT"); v != "" {
		c.Inference.Endpoint = v
	}
}
func (c Config) InferenceTimeout() time.Duration {
	return time.Duration(c.Inference.TimeoutSeconds) * time.Second
}
