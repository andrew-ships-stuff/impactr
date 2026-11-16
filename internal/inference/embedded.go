package inference

import (
	"context"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/progress"
	llama "github.com/goccy/go-llama"
	"os"
	"path/filepath"
	"time"
)

type Embedded struct {
	ModelPath                           string
	Threads, ContextLength, MaxMemoryMB int
	ModelName                           string
	Report                              func(progress.Event)
}

func (e Embedded) report(ev progress.Event) {
	if e.Report != nil {
		e.Report(ev)
	}
}

const explanationGrammar = `root ::= ws "{" ws "\"summary\"" ws ":" ws string ws "," ws "\"claims\"" ws ":" ws "[" ws claims? ws "]" ws "}" ws
claims ::= claim
claim ::= "{" ws "\"text\"" ws ":" ws string ws "," ws "\"evidence_ids\"" ws ":" ws "[" ws ids? ws "]" ws "," ws "\"classification\"" ws ":" ws classification ws "}"
ids ::= string (ws "," ws string)*
classification ::= "\"resolved\"" | "\"potential\"" | "\"unresolved\""
string ::= "\"" ([^"\\] | "\\" ["\\/bfnrt])* "\""
ws ::= [ \t\n\r]{0,2}`

const briefGrammar = `root ::= ws "{" ws "\"headline\"" ws ":" ws string ws "," ws "\"clusters\"" ws ":" ws "[" ws clusters? ws "]" ws "}" ws
clusters ::= cluster (ws "," ws cluster)*
cluster ::= "{" ws "\"package\"" ws ":" ws string ws "," ws "\"title\"" ws ":" ws string ws "}"
string ::= "\"" ([^"\\] | "\\" ["\\/bfnrt])* "\""
ws ::= [ \t\n\r]{0,2}`

func (Embedded) Name() string { return "embedded" }

// chat wraps the prompt in Qwen ChatML so the instruct checkpoint behaves;
// bare prompts without the template derail small models badly.
func chat(user string) string {
	const imStart, imEnd = "<|" + "im_start" + "|>", "<|" + "im_end" + "|>"
	return imStart + "system\nAnswer with one JSON object only.\n" + imEnd + "\n" +
		imStart + "user\n" + user + "\n" + imEnd + "\n" + imStart + "assistant\n"
}

// generate loads the model, produces grammar-constrained JSON, and honors
// context cancellation by interrupting generation. Loader and token activity
// surface through Report when set.
func (e Embedded) generate(ctx context.Context, prompt string, maxTokens int, grammar, phase string) (string, error) {
	if e.ModelPath == "" {
		return "", fmt.Errorf("embedded model is not installed; run impactr model install")
	}
	mem := e.MaxMemoryMB
	if mem <= 0 {
		mem = 2048
	}
	nctx := e.ContextLength
	if nctx <= 0 {
		nctx = 2048
	}
	threads := e.Threads
	if threads <= 0 {
		threads = 1
	}
	size := int64(0)
	if fi, err := os.Stat(e.ModelPath); err == nil {
		size = fi.Size()
	}
	e.report(progress.Event{Phase: "model", Detail: fmt.Sprintf("loading %s (%d MiB, cpu, %d MiB budget)", filepath.Base(e.ModelPath), size>>20, mem)})
	loadStart := time.Now()
	engine, err := llama.New(llama.WithPreopenDir(filepath.Dir(e.ModelPath)), llama.WithMaxMemory(uint64(mem)<<20))
	if err != nil {
		e.report(progress.Event{Phase: "model", Detail: "runtime init failed", Done: true, Err: err, Elapsed: time.Since(loadStart)})
		return "", err
	}
	defer engine.Close()
	model, err := engine.LoadModel(filepath.Base(e.ModelPath))
	if err != nil {
		e.report(progress.Event{Phase: "model", Detail: "model load failed", Done: true, Err: err, Elapsed: time.Since(loadStart)})
		return "", err
	}
	defer model.Close()
	lc, err := model.NewContext(llama.ContextParams{NCtx: uint32(nctx), NThreads: uint32(threads)})
	if err != nil {
		e.report(progress.Event{Phase: "model", Detail: "context alloc failed", Done: true, Err: err, Elapsed: time.Since(loadStart)})
		return "", err
	}
	defer lc.Close()
	e.report(progress.Event{Phase: "model", Detail: fmt.Sprintf("ready · n_ctx %d · %d thread(s) · %d MiB budget", nctx, threads, mem), Done: true, Elapsed: time.Since(loadStart)})
	wrapped := chat(prompt)
	if est := len(wrapped)/3 + maxTokens; est > nctx {
		return "", fmt.Errorf("prompt too large for n_ctx %d (≈%d tokens); lower analysis.max_nodes or inference.max_output_tokens", nctx, est)
	}
	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	genStart := time.Now()
	e.report(progress.Event{Phase: phase, Detail: "generating (grammar-constrained, temp 0)"})
	go func() {
		pieces, last := 0, time.Now()
		got, er := lc.Stream(wrapped, llama.Params{NPredict: maxTokens, Temperature: 0, Seed: 1, Grammar: grammar}, func(piece string) {
			pieces++
			if pieces%16 == 0 && time.Since(last) > 150*time.Millisecond {
				last = time.Now()
				e.report(progress.Event{Phase: phase, Detail: fmt.Sprintf("%d pieces · %.1f pieces/s", pieces, float64(pieces)/time.Since(genStart).Seconds())})
			}
		})
		e.report(progress.Event{Phase: phase, Detail: fmt.Sprintf("%d pieces in %.1fs (%.1f pieces/s)", pieces, time.Since(genStart).Seconds(), float64(pieces)/max(time.Since(genStart).Seconds(), 0.001)), Done: true, Err: er, Elapsed: time.Since(genStart)})
		ch <- result{got.Text, er}
	}()
	select {
	case <-ctx.Done():
		lc.Interrupt()
		<-ch
		return "", ctx.Err()
	case got := <-ch:
		return got.text, got.err
	}
}

func (e Embedded) Explain(ctx context.Context, r Request) (Explanation, error) {
	text, err := e.generate(ctx, Prompt(r), r.MaxTokens, explanationGrammar, "explain")
	if err != nil {
		return Explanation{}, err
	}
	out, er := Parse(text, r)
	out.Provider = "embedded"
	out.Model = e.ModelName
	return out, er
}

func (e Embedded) Brief(ctx context.Context, r BriefRequest) (Brief, error) {
	tokens := r.MaxTokens
	if tokens <= 0 {
		tokens = 256
	}
	text, err := e.generate(ctx, BriefPrompt(r), tokens, briefGrammar, "brief")
	if err != nil {
		return Brief{}, err
	}
	out, er := ParseBrief(text, r)
	out.Provider = "embedded"
	out.Model = e.ModelName
	return out, er
}
