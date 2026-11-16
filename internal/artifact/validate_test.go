package artifact

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

func TestRejectsSymlink(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "target")
	_ = os.WriteFile(target, []byte("x"), 0600)
	if err := os.Symlink(target, filepath.Join(d, "link")); err != nil {
		t.Skip(err)
	}
	if Validate(d) == nil {
		t.Fatal("expected rejection")
	}
}

func validBundle(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	envelope := struct {
		SchemaVersion string `json:"schema_version"`
		After         struct {
			Provenance graph.Provenance `json:"provenance"`
		} `json:"after"`
	}{SchemaVersion: graph.SchemaVersion}
	envelope.After.Provenance.BaseSHA = "base"
	envelope.After.Provenance.HeadSHA = "head"
	b, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(d, "analysis.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(d, "summary.md"), []byte("<!-- impactr-comment:v1 -->"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(d, "impact.png"))
	if err != nil {
		t.Fatal(err)
	}
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.Black)
	if err = png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestValidateExpectedBindsArtifactToPullRequest(t *testing.T) {
	d := validBundle(t)
	if err := ValidateExpected(d, Expectations{BaseSHA: "base", HeadSHA: "head"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExpected(d, Expectations{BaseSHA: "base", HeadSHA: "other"}); err == nil {
		t.Fatal("expected head SHA mismatch")
	}
}

func TestValidateChecksEveryImpactImage(t *testing.T) {
	d := validBundle(t)
	if err := os.WriteFile(filepath.Join(d, "impact-1.png"), []byte("not a png"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(d); err == nil {
		t.Fatal("expected malformed cluster image rejection")
	}
}
