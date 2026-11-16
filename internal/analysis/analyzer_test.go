package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGoCrossPackageImportAndCall(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(root, name)
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/app\n")
	write("service/service.go", "package service\nfunc Validate() bool { return true }\n")
	write("handler/handler.go", "package handler\nimport \"example.test/app/service\"\nfunc Handle() { service.Validate() }\n")
	g, err := New().Analyze(context.Background(), root, "test", Options{Languages: []string{"go"}, Workers: 2, CacheDir: filepath.Join(root, ".impactr", "cache")})
	if err != nil {
		t.Fatal(err)
	}
	imports, calls := 0, 0
	for _, e := range g.Edges {
		if e.Kind == "imports" && e.Resolution == "resolved" {
			imports++
		}
		if e.Kind == "calls" && e.Resolution == "resolved" {
			calls++
		}
	}
	if imports != 1 || calls != 1 {
		t.Fatalf("imports=%d calls=%d edges=%+v", imports, calls, g.Edges)
	}
}

func TestGoInterfaceAndHTTPRoute(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/web\n"), 0644)
	src := `package web
import "net/http"
type Server interface { Serve() }
type App struct{}
func (App) Serve() {}
func Handler(http.ResponseWriter, *http.Request) {}
func Routes() { http.HandleFunc("/health", Handler) }
`
	_ = os.WriteFile(filepath.Join(root, "web.go"), []byte(src), 0644)
	g, err := New().Analyze(context.Background(), root, "test", Options{Languages: []string{"go"}, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	implements, routes := 0, 0
	for _, e := range g.Edges {
		if e.Kind == "implements" {
			implements++
		}
		if e.Kind == "handles_route" {
			routes++
		}
	}
	if implements != 1 || routes != 1 {
		t.Fatalf("implements=%d routes=%d edges=%+v", implements, routes, g.Edges)
	}
}
