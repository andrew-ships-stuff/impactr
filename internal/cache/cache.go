package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

type Store struct{ Dir string }

func (s Store) Load(key string) (graph.Graph, bool, error) {
	if s.Dir == "" {
		return graph.Graph{}, false, nil
	}
	if !safe(key) {
		return graph.Graph{}, false, fmt.Errorf("unsafe cache key")
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return graph.Graph{}, false, nil
	}
	if err != nil {
		return graph.Graph{}, false, err
	}
	var g graph.Graph
	if err = json.Unmarshal(b, &g); err != nil {
		return g, false, err
	}
	if g.SchemaVersion != graph.SchemaVersion {
		return graph.Graph{}, false, nil
	}
	return g, true, nil
}

func (s Store) Save(key string, g graph.Graph) error {
	if s.Dir == "" {
		return nil
	}
	if !safe(key) {
		return fmt.Errorf("unsafe cache key")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(g)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, key+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, key+".json"))
}

func safe(s string) bool { return s != "" && !strings.ContainsAny(s, "/\\.") }
