package inference

import (
	"context"
	"fmt"
	llama "github.com/goccy/go-llama"
	"path/filepath"
)

type Embedded struct {
	ModelPath                           string
	Threads, ContextLength, MaxMemoryMB int
	ModelName                           string
}

const explanationGrammar = `root ::= ws "{" ws "\"summary\"" ws ":" ws string ws "," ws "\"claims\"" ws ":" ws "[" ws claims? ws "]" ws "}" ws
claims ::= claim
claim ::= "{" ws "\"text\"" ws ":" ws string ws "," ws "\"evidence_ids\"" ws ":" ws "[" ws ids? ws "]" ws "," ws "\"classification\"" ws ":" ws classification ws "}"
ids ::= string (ws "," ws string)*
classification ::= "\"resolved\"" | "\"potential\"" | "\"unresolved\""
string ::= "\"" ([^"\\] | "\\" ["\\/bfnrt])* "\""
ws ::= [ \t\n\r]*`

func (Embedded) Name() string { return "embedded" }
func (e Embedded) Explain(ctx context.Context, r Request) (Explanation, error) {
	if e.ModelPath == "" {
		return Explanation{}, fmt.Errorf("embedded model is not installed; run impactr model install")
	}
	mem := e.MaxMemoryMB
	if mem <= 0 {
		mem = 2048
	}
	engine, err := llama.New(llama.WithPreopenDir(filepath.Dir(e.ModelPath)), llama.WithMaxMemory(uint64(mem)<<20))
	if err != nil {
		return Explanation{}, err
	}
	defer engine.Close()
	model, err := engine.LoadModel(filepath.Base(e.ModelPath))
	if err != nil {
		return Explanation{}, err
	}
	defer model.Close()
	nctx := e.ContextLength
	if nctx <= 0 {
		nctx = 2048
	}
	threads := e.Threads
	if threads <= 0 {
		threads = 1
	}
	lc, err := model.NewContext(llama.ContextParams{NCtx: uint32(nctx), NThreads: uint32(threads)})
	if err != nil {
		return Explanation{}, err
	}
	defer lc.Close()
	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		got, er := lc.Generate(Prompt(r), llama.Params{NPredict: r.MaxTokens, Temperature: 0, Seed: 1, Grammar: explanationGrammar})
		ch <- result{got.Text, er}
	}()
	select {
	case <-ctx.Done():
		lc.Interrupt()
		<-ch
		return Explanation{}, ctx.Err()
	case got := <-ch:
		if got.err != nil {
			return Explanation{}, got.err
		}
		out, er := Parse(got.text, r)
		out.Provider = "embedded"
		out.Model = e.ModelName
		return out, er
	}
}
