package artifact

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

const MaxTotalSize int64 = 25 << 20

type Expectations struct {
	BaseSHA string
	HeadSHA string
}

func Validate(dir string) error { return ValidateExpected(dir, Expectations{}) }

func ValidateExpected(dir string, want Expectations) error {
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
		After         struct {
			Provenance graph.Provenance `json:"provenance"`
		} `json:"after"`
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		return fmt.Errorf("invalid analysis JSON: %w", err)
	}
	if envelope.SchemaVersion != graph.SchemaVersion {
		return fmt.Errorf("unsupported schema %q", envelope.SchemaVersion)
	}
	if want.BaseSHA != "" && envelope.After.Provenance.BaseSHA != want.BaseSHA {
		return fmt.Errorf("artifact base SHA mismatch: got %s", envelope.After.Provenance.BaseSHA)
	}
	if want.HeadSHA != "" && envelope.After.Provenance.HeadSHA != want.HeadSHA {
		return fmt.Errorf("artifact head SHA mismatch: got %s", envelope.After.Provenance.HeadSHA)
	}
	images, err := filepath.Glob(filepath.Join(dir, "impact*.png"))
	if err != nil {
		return err
	}
	if len(images) == 0 {
		return fmt.Errorf("missing impact image")
	}
	for _, path := range images {
		base := filepath.Base(path)
		if base != "impact.png" && !validClusterImage(base) {
			return fmt.Errorf("unexpected impact image %q", base)
		}
		if err = validatePNG(path); err != nil {
			return err
		}
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

func validClusterImage(name string) bool {
	var n int
	count, err := fmt.Sscanf(name, "impact-%d.png", &n)
	return strings.HasSuffix(name, ".png") && err == nil && count == 1 && n >= 1 && n <= 16
}

func validatePNG(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	cfg, err := png.DecodeConfig(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("invalid %s: %w", filepath.Base(path), err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return fmt.Errorf("unsafe image dimensions in %s", filepath.Base(path))
	}
	f, err = os.Open(path)
	if err != nil {
		return err
	}
	_, _, err = image.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("invalid %s payload: %w", filepath.Base(path), err)
	}
	return nil
}
