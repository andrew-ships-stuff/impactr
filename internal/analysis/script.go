package analysis

import (
	"bufio"
	"context"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ScriptAnalyzer struct {
	language   string
	extensions []string
}

func (s ScriptAnalyzer) Name() string         { return s.language }
func (s ScriptAnalyzer) Version() string      { return "syntax/v1" }
func (s ScriptAnalyzer) Extensions() []string { return s.extensions }

var jsImport = regexp.MustCompile(`(?:from\s+|require\s*\(\s*)['"]([^'"]+)`)
var pyImport = regexp.MustCompile(`^(?:from|import)\s+([A-Za-z0-9_\.]+)`)
var scriptFunc = regexp.MustCompile(`(?:function\s+|def\s+|(?:const|let|var)\s+)([A-Za-z_][A-Za-z0-9_]*)`)

func (s ScriptAnalyzer) Analyze(ctx context.Context, root string, files []string) ([]graph.Node, []graph.Edge, []string, error) {
	var ns []graph.Node
	var es []graph.Edge
	for _, p := range files {
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		default:
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		mid := graph.StableNodeID(s.language, "", rel, "module")
		ns = append(ns, graph.Node{ID: mid, Kind: "module", Name: filepath.Base(rel), QualifiedName: rel, Language: s.language, Location: graph.Location{Path: rel}, Resolution: graph.Resolved})
		f, e := os.Open(p)
		if e != nil {
			return nil, nil, nil, e
		}
		scan := bufio.NewScanner(f)
		line := 0
		for scan.Scan() {
			line++
			txt := strings.TrimSpace(scan.Text())
			re := jsImport
			if s.language == "python" {
				re = pyImport
			}
			if m := re.FindStringSubmatch(txt); len(m) > 1 {
				dep := m[1]
				did := graph.StableNodeID(s.language, "", dep, "module")
				ns = append(ns, graph.Node{ID: did, Kind: "external_dependency", Name: filepath.Base(dep), QualifiedName: dep, Language: s.language, Resolution: graph.Unresolved})
				es = append(es, edge(mid, did, "imports", graph.Unresolved, rel, line, "import "+dep))
			}
			if m := scriptFunc.FindStringSubmatch(txt); len(m) > 1 {
				q := rel + "." + m[1]
				id := graph.StableNodeID(s.language, rel, q, "function")
				ns = append(ns, graph.Node{ID: id, Kind: "function", Name: m[1], QualifiedName: q, Language: s.language, Location: graph.Location{Path: rel, StartLine: line, EndLine: line}, BodyHash: graph.Hash([]byte(txt)), Resolution: graph.Resolved, Evidence: []graph.Evidence{{Kind: "declaration", Location: graph.Location{Path: rel, StartLine: line, EndLine: line}, Detail: "syntax-level function declaration"}}})
				es = append(es, edge(mid, id, "contains", graph.Resolved, rel, line, "contains function"))
			}
		}
		e = scan.Err()
		f.Close()
		if e != nil {
			return nil, nil, nil, e
		}
	}
	return dedupeNodes(ns), dedupeEdges(es), []string{s.language + " analysis is syntax-level; dynamic calls are not resolved"}, nil
}
