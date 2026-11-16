package artifact

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

const MaxTotalSize int64 = 25 << 20

func Validate(dir string) error {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", path)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular artifact: %s", path)
		}
		total += info.Size()
		if total > MaxTotalSize {
			return fmt.Errorf("artifact exceeds %d bytes", MaxTotalSize)
		}
		return nil
	})
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, "analysis.json"))
	if err != nil {
		return err
	}
	if len(b) > 10<<20 {
		return fmt.Errorf("analysis.json is too large")
	}
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		return fmt.Errorf("invalid analysis JSON: %w", err)
	}
	if envelope.SchemaVersion != graph.SchemaVersion {
		return fmt.Errorf("unsupported schema %q", envelope.SchemaVersion)
	}
	f, err := os.Open(filepath.Join(dir, "impact.png"))
	if err != nil {
		return err
	}
	cfg, format, err := image.DecodeConfig(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("invalid impact image: %w", err)
	}
	if format != "png" || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return fmt.Errorf("unsafe image dimensions or format")
	}
	md, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(md), "<!-- impactr-comment:v1 -->") {
		return fmt.Errorf("missing Impactr comment marker")
	}
	return nil
}
