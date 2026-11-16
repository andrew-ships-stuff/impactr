package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"strings"
)

// ClusterBrief is an LLM-authored label for one deterministic change cluster.
// The model never chooses which clusters exist; it only titles rollups that
// the static analysis already found, and unknown packages are dropped.
type ClusterBrief struct {
	Package string `json:"package"`
	Title   string `json:"title"`
}

type Brief struct {
	Headline string         `json:"headline"`
	Clusters []ClusterBrief `json:"clusters,omitempty"`
	Provider string         `json:"provider"`
	Model    string         `json:"model,omitempty"`
	Fallback bool           `json:"fallback"`
}

type BriefRequest struct {
	Views     []graph.PackageView
	Clusters  []graph.Cluster
	MaxTokens int
}

// Briefer is an optional provider capability: summarize deterministic rollups
// into a headline and per-cluster titles for rendering.
type Briefer interface {
	Brief(context.Context, BriefRequest) (Brief, error)
}

// Narrator can produce both AI-authored outputs while sharing expensive
// provider state such as an embedded model load. Providers that do not
// implement it continue to use Explain and Brief independently.
type Narrator interface {
	Narrate(context.Context, Request, BriefRequest) (Explanation, Brief, error)
}

func BriefFacts(req BriefRequest) []byte {
	type pkgFact struct {
		Package  string `json:"package"`
		Added    int    `json:"added,omitempty"`
		Removed  int    `json:"removed,omitempty"`
		Modified int    `json:"modified,omitempty"`
		Affected int    `json:"affected,omitempty"`
		Blast    int    `json:"blast,omitempty"`
	}
	var facts struct {
		Packages []pkgFact `json:"packages"`
	}
	for _, v := range req.Views {
		facts.Packages = append(facts.Packages, pkgFact{Package: v.Package, Added: v.Added, Removed: v.Removed, Modified: v.Modified, Affected: v.Affected, Blast: v.Blast})
	}
	b, _ := json.Marshal(facts)
	return b
}

func BriefPrompt(req BriefRequest) string {
	known := make([]string, 0, len(req.Clusters))
	for _, c := range req.Clusters {
		known = append(known, c.Package)
	}
	return fmt.Sprintf(`You are summarizing a deterministic architecture-impact rollup. Using ONLY the JSON facts, output one JSON object with keys "headline" and "clusters". "headline" is one concrete sentence naming the most important changed package or symbol; never instructions or placeholders. "clusters" is an array of {"package": string, "title": string} covering at most %d of these exact packages: %s. Titles are short phrases describing what the change affects; never invent packages, symbols, or dependencies. Facts: %s`,
		len(known), strings.Join(known, ", "), string(BriefFacts(req)))
}

func ParseBrief(raw string, req BriefRequest) (Brief, error) {
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return Brief{}, fmt.Errorf("provider returned no JSON object")
	}
	var b Brief
	if e := json.Unmarshal([]byte(raw[start:end+1]), &b); e != nil {
		return b, e
	}
	return ValidateBrief(b, req)
}

func ValidateBrief(b Brief, req BriefRequest) (Brief, error) {
	known := map[string]bool{}
	for _, c := range req.Clusters {
		known[c.Package] = true
	}
	seen := map[string]bool{}
	out := b.Clusters[:0]
	for _, c := range b.Clusters {
		if known[c.Package] && !seen[c.Package] && strings.TrimSpace(c.Title) != "" {
			seen[c.Package] = true
			out = append(out, c)
		}
	}
	b.Clusters = out
	headline := strings.TrimSpace(b.Headline)
	if headline == "" {
		return b, fmt.Errorf("empty brief headline")
	}
	if len(headline) > 240 || strings.Count(headline, ",") > 6 {
		return b, fmt.Errorf("brief headline is not concise")
	}
	nonASCII := 0
	for _, r := range headline {
		if r > 127 {
			nonASCII++
		}
	}
	if nonASCII > len([]rune(headline))/10 {
		return b, fmt.Errorf("brief headline is not in the requested language")
	}
	grounded := len(known) == 0
	for p := range known {
		base := p
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		if strings.Contains(headline, p) || (base != "" && strings.Contains(headline, base)) {
			grounded = true
			break
		}
	}
	if !grounded {
		return b, fmt.Errorf("brief headline does not name an analyzed package")
	}
	return b, nil
}

func DeterministicBrief(req BriefRequest, _ string) Brief {
	changed, affected, packages, top := 0, 0, 0, ""
	var largest *graph.PackageView
	for _, v := range req.Views {
		if v.External {
			continue
		}
		localChanged := v.Added + v.Removed + v.Modified
		changed += localChanged
		affected += v.Affected
		if localChanged > 0 {
			packages++
		}
		if largest == nil || v.Affected > largest.Affected || (v.Affected == largest.Affected && localChanged > largest.Added+largest.Removed+largest.Modified) {
			copy := v
			largest = &copy
		}
	}
	if largest != nil && largest.Affected > 0 {
		top = fmt.Sprintf(" Review %s first: %d downstream symbols may depend on its changes.", largest.Package, largest.Affected)
	}
	b := Brief{
		Headline: fmt.Sprintf("%d symbols changed across %d local packages; %d downstream symbols may be affected.%s", changed, packages, affected, top),
		Provider: "deterministic",
		Fallback: true,
	}
	for _, c := range req.Clusters {
		changed, affected := 0, 0
		for _, v := range req.Views {
			if v.Package == c.Package {
				changed, affected = v.Added+v.Removed+v.Modified, v.Affected
			}
		}
		b.Clusters = append(b.Clusters, ClusterBrief{Package: c.Package, Title: fmt.Sprintf("%d changed, %d potentially affected", changed, affected)})
	}
	return b
}
