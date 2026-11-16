package analysis

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

type GoAnalyzer struct{}

func (GoAnalyzer) Name() string         { return "go" }
func (GoAnalyzer) Version() string      { return "go-ast/v1" }
func (GoAnalyzer) Extensions() []string { return []string{".go"} }

type goFile struct {
	path, pkg, pkgID string
	f                *ast.File
	fs               *token.FileSet
	imports          map[string]string
}
type interfaceFact struct {
	id      string
	methods map[string]string
	path    string
	line    int
}
type methodFact struct {
	signature string
	pointer   bool
}

func (GoAnalyzer) Analyze(ctx context.Context, root string, files []string) ([]graph.Node, []graph.Edge, []string, error) {
	module := modulePath(root)
	var parsed []goFile
	var nodes []graph.Node
	var edges []graph.Edge
	symbols := map[string]string{}
	packages := map[string]bool{}
	interfaces := map[string]interfaceFact{}
	methodSets := map[string]map[string]methodFact{}
	structs := map[string]string{}
	for _, path := range files {
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		default:
		}
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, path, nil, parser.ParseComments)
		if err != nil {
			return nil, nil, nil, err
		}
		rel, _ := filepath.Rel(root, path)
		dir := filepath.ToSlash(filepath.Dir(rel))
		pkg := module
		if dir != "." {
			pkg += "/" + dir
		}
		pid := graph.StableNodeID("go", module, pkg, "package")
		if !packages[pid] {
			packages[pid] = true
			nodes = append(nodes, graph.Node{ID: pid, Kind: "package", Name: f.Name.Name, QualifiedName: pkg, Language: "go", Location: graph.Location{Path: dir}, Resolution: graph.Resolved})
		}
		im := map[string]string{}
		for _, x := range f.Imports {
			p, _ := strconv.Unquote(x.Path.Value)
			name := filepath.Base(p)
			if x.Name != nil {
				name = x.Name.Name
			}
			im[name] = p
			to := graph.StableNodeID("go", module, p, "package")
			res := graph.Resolved
			depKind := "package"
			if !strings.HasPrefix(p, module) {
				if strings.Contains(strings.Split(p, "/")[0], ".") {
					res = graph.Unresolved
					depKind = "external_dependency"
				}
				nodes = append(nodes, graph.Node{ID: to, Kind: depKind, Name: filepath.Base(p), QualifiedName: p, Language: "go", Resolution: res})
			}
			edges = append(edges, edge(pid, to, "imports", res, rel, fs.Position(x.Pos()).Line, "import "+p))
		}
		parsed = append(parsed, goFile{path: rel, pkg: pkg, pkgID: pid, f: f, fs: fs, imports: im})
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.GenDecl:
				for _, s := range x.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						kind := "type"
						switch ts.Type.(type) {
						case *ast.StructType:
							kind = "struct"
						case *ast.InterfaceType:
							kind = "interface"
						}
						q := pkg + "." + ts.Name.Name
						id := graph.StableNodeID("go", pkg, q, kind)
						symbols[q] = id
						nodes = append(nodes, node(id, kind, ts.Name.Name, q, ast.IsExported(ts.Name.Name), rel, fs, x, ts.Type))
						if kind == "struct" {
							structs[q] = id
						}
						if it, ok := ts.Type.(*ast.InterfaceType); ok {
							methods := map[string]string{}
							for _, field := range it.Methods.List {
								ft, ok := field.Type.(*ast.FuncType)
								if !ok {
									continue
								}
								sig := printNode(fs, ft)
								for _, name := range field.Names {
									methods[name.Name] = sig
								}
							}
							interfaces[q] = interfaceFact{id: id, methods: methods, path: rel, line: fs.Position(ts.Pos()).Line}
						}
					}
				}
			case *ast.FuncDecl:
				q := pkg + "." + x.Name.Name
				kind := "function"
				if x.Recv != nil {
					recv := receiver(x)
					q = pkg + "." + recv + "." + x.Name.Name
					kind = "method"
					set := methodSets[pkg+"."+recv]
					if set == nil {
						set = map[string]methodFact{}
						methodSets[pkg+"."+recv] = set
					}
					_, pointer := x.Recv.List[0].Type.(*ast.StarExpr)
					set[x.Name.Name] = methodFact{signature: printNode(fs, x.Type), pointer: pointer}
				}
				id := graph.StableNodeID("go", pkg, q, kind)
				symbols[q] = id
				nodes = append(nodes, node(id, kind, x.Name.Name, q, ast.IsExported(x.Name.Name), rel, fs, x, x.Type))
			}
		}
	}
	for structQ, structID := range structs {
		for _, iface := range interfaces {
			if len(iface.methods) == 0 {
				continue
			}
			set := methodSets[structQ]
			matches := true
			potential := false
			for name, sig := range iface.methods {
				m, ok := set[name]
				if !ok || m.signature != sig {
					matches = false
					break
				}
				potential = potential || m.pointer
			}
			if matches {
				res := graph.Resolved
				if potential {
					res = graph.Potential
				}
				edges = append(edges, edge(structID, iface.id, "implements", res, iface.path, iface.line, "static Go method-set match"))
			}
		}
	}
	// Resolve direct calls after all declarations are indexed.
	for _, pf := range parsed {
		ast.Inspect(pf.f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			from := enclosingSymbol(pf, nodes, pf.fs.Position(call.Pos()).Line)
			if from == "" {
				return true
			}
			var q string
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Obj != nil && fun.Obj.Kind == ast.Var {
					return true
				}
				if goBuiltin(fun.Name) {
					return true
				}
				q = pf.pkg + "." + fun.Name
			case *ast.SelectorExpr:
				if id, ok := fun.X.(*ast.Ident); ok {
					if imp := pf.imports[id.Name]; imp != "" {
						q = imp + "." + fun.Sel.Name
					}
				}
			}
			if (strings.HasSuffix(q, ".HandleFunc") || strings.HasSuffix(q, ".Handle")) && len(call.Args) >= 2 {
				if route, ok := call.Args[0].(*ast.BasicLit); ok && route.Kind == token.STRING {
					path, _ := strconv.Unquote(route.Value)
					endpointID := graph.StableNodeID("go", pf.pkg, "http:"+path, "http_endpoint")
					nodes = append(nodes, graph.Node{ID: endpointID, Kind: "http_endpoint", Name: path, QualifiedName: "HTTP " + path, Language: "go", Location: graph.Location{Path: pf.path, StartLine: pf.fs.Position(route.Pos()).Line, EndLine: pf.fs.Position(route.End()).Line}, Resolution: graph.Resolved, Evidence: []graph.Evidence{{Kind: "route_registration", Location: graph.Location{Path: pf.path, StartLine: pf.fs.Position(route.Pos()).Line}, Detail: "static HTTP route registration"}}})
					handlerQ := ""
					switch h := call.Args[1].(type) {
					case *ast.Ident:
						handlerQ = pf.pkg + "." + h.Name
					case *ast.SelectorExpr:
						if id, ok := h.X.(*ast.Ident); ok {
							if imp := pf.imports[id.Name]; imp != "" {
								handlerQ = imp + "." + h.Sel.Name
							}
						}
					}
					if handlerID := symbols[handlerQ]; handlerID != "" {
						edges = append(edges, edge(handlerID, endpointID, "handles_route", graph.Resolved, pf.path, pf.fs.Position(call.Pos()).Line, "registered handler for "+path))
					}
				}
			}
			to := symbols[q]
			res := graph.Resolved
			if to == "" {
				if q == "" {
					return true
				}
				to = graph.StableNodeID("go", "", q, "function")
				res = graph.Unresolved
				nodes = append(nodes, graph.Node{ID: to, Kind: "function", Name: filepath.Base(q), QualifiedName: q, Language: "go", Resolution: graph.Unresolved})
			}
			line := pf.fs.Position(call.Pos()).Line
			edges = append(edges, edge(from, to, "calls", res, pf.path, line, "call "+q))
			return true
		})
	}
	nodes = dedupeNodes(nodes)
	edges = dedupeEdges(edges)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, edges, nil, nil
}
func goBuiltin(name string) bool {
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover", "string", "bool", "byte", "rune", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128", "error", "any":
		return true
	}
	return false
}
func modulePath(root string) string {
	b, e := os.ReadFile(filepath.Join(root, "go.mod"))
	if e == nil {
		f := strings.Fields(string(b))
		for i := range f {
			if f[i] == "module" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return filepath.Base(root)
}
func receiver(f *ast.FuncDecl) string {
	if f.Recv == nil || len(f.Recv.List) == 0 {
		return ""
	}
	switch x := f.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		if i, ok := x.X.(*ast.Ident); ok {
			return i.Name
		}
	}
	return "?"
}
func printNode(fs *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fs, n)
	return b.String()
}
func node(id, kind, name, q string, exported bool, path string, fs *token.FileSet, whole ast.Node, sig ast.Node) graph.Node {
	var sb, bb bytes.Buffer
	_ = printer.Fprint(&sb, fs, sig)
	_ = printer.Fprint(&bb, fs, whole)
	start, end := fs.Position(whole.Pos()).Line, fs.Position(whole.End()).Line
	return graph.Node{ID: id, Kind: kind, Name: name, QualifiedName: q, Language: "go", Location: graph.Location{Path: path, StartLine: start, EndLine: end}, Exported: exported, Signature: sb.String(), BodyHash: graph.Hash(bb.Bytes()), Resolution: graph.Resolved, Evidence: []graph.Evidence{{Kind: "declaration", Location: graph.Location{Path: path, StartLine: start, EndLine: end}, Detail: fmt.Sprintf("%s declaration", kind)}}}
}
func edge(from, to, kind string, res graph.Resolution, path string, line int, detail string) graph.Edge {
	return graph.Edge{ID: graph.StableEdgeID(from, to, kind), From: from, To: to, Kind: kind, Resolution: res, Evidence: []graph.Evidence{{Kind: "syntax", Location: graph.Location{Path: path, StartLine: line, EndLine: line}, Detail: detail}}}
}
func enclosingSymbol(pf goFile, nodes []graph.Node, line int) string {
	best := ""
	span := int(^uint(0) >> 1)
	for _, n := range nodes {
		if n.Location.Path == pf.path && line >= n.Location.StartLine && line <= n.Location.EndLine && (n.Kind == "function" || n.Kind == "method") {
			s := n.Location.EndLine - n.Location.StartLine
			if s < span {
				best = n.ID
				span = s
			}
		}
	}
	return best
}
func dedupeNodes(in []graph.Node) []graph.Node {
	m := map[string]graph.Node{}
	for _, n := range in {
		if old, ok := m[n.ID]; !ok || old.Resolution == graph.Unresolved {
			m[n.ID] = n
		}
	}
	out := make([]graph.Node, 0, len(m))
	for _, n := range m {
		out = append(out, n)
	}
	return out
}
func dedupeEdges(in []graph.Edge) []graph.Edge {
	m := map[string]graph.Edge{}
	for _, e := range in {
		if old, ok := m[e.ID]; ok {
			old.Evidence = append(old.Evidence, e.Evidence...)
			m[e.ID] = old
		} else {
			m[e.ID] = e
		}
	}
	out := make([]graph.Edge, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	return out
}
