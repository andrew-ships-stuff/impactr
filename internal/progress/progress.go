// Package progress carries human-readable lifecycle events from analysis
// phases to a terminal (or any) sink. It never affects results; consumers
// treat it as advisory.
package progress

import (
	"sync"
	"time"
)

type Event struct {
	Phase   string
	Detail  string
	Done    bool
	Err     error
	Elapsed time.Duration
}

type Reporter struct {
	mu   sync.Mutex
	sink func(Event)
}

func New(sink func(Event)) *Reporter { return &Reporter{sink: sink} }

func (r *Reporter) Emit(e Event) {
	if r == nil || r.sink == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sink(e)
}

// Phase emits a start event, runs fn, then a done event with timing.
func (r *Reporter) Phase(phase, start string, fn func() (string, error)) error {
	if r == nil {
		_, err := fn()
		return err
	}
	r.Emit(Event{Phase: phase, Detail: start})
	t := time.Now()
	detail, err := fn()
	r.Emit(Event{Phase: phase, Detail: detail, Done: true, Err: err, Elapsed: time.Since(t)})
	return err
}
