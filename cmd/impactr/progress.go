package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/andrew-ships-stuff/impactr/internal/progress"
)

const (
	reset = "\033[0m"
	dim   = "\033[2m"
	cyan  = "\033[36m"
	green = "\033[32m"
	red   = "\033[31m"
)

// newTerminalReporter renders live phase progress on stderr: a braille-free
// spinner per running phase, replaced by an OK/XX line with timing on
// completion. Falls back to one plain line per completed phase when stdout
// is not a TTY (CI) or NO_COLOR/TERM=dumb is set.
func newTerminalReporter() *progress.Reporter {
	out := os.Stderr
	fi, err := out.Stat()
	tty := err == nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
	color := tty && os.Getenv("NO_COLOR") == ""
	paintColor := func(s, c string) string {
		if !color {
			return s
		}
		return c + s + reset
	}
	type line struct {
		detail  string
		running bool
		elapsed time.Duration
		err     error
	}
	var mu sync.Mutex
	order := []string{}
	lines := map[string]*line{}
	rendered := 0
	frames := []string{`|`, `/`, `-`, `\`}
	frame := 0
	paint := func() {
		if rendered > 0 {
			fmt.Fprintf(out, "\033[%dF", rendered)
		}
		for _, ph := range order {
			l := lines[ph]
			mark := paintColor("ok", green)
			switch {
			case l.err != nil:
				mark = paintColor("XX", red)
			case l.running:
				mark = paintColor(frames[frame%len(frames)], cyan)
			}
			s := fmt.Sprintf(" %s %-14s %s", mark, ph, l.detail)
			if !l.running && l.elapsed > 0 {
				s += " " + paintColor(l.elapsed.Round(10*time.Millisecond).String(), dim)
			}
			fmt.Fprint(out, s+"\033[K\n")
		}
		rendered = len(order)
	}
	if tty {
		go func() {
			for range time.NewTicker(90 * time.Millisecond).C {
				mu.Lock()
				running := false
				for _, l := range lines {
					if l.running {
						running = true
						break
					}
				}
				if running {
					frame++
					paint()
				}
				mu.Unlock()
			}
		}()
	}
	return progress.New(func(ev progress.Event) {
		mu.Lock()
		defer mu.Unlock()
		l, ok := lines[ev.Phase]
		if !ok {
			l = &line{}
			lines[ev.Phase] = l
			order = append(order, ev.Phase)
		}
		if ev.Detail != "" {
			l.detail = ev.Detail
		}
		if ev.Done {
			l.running = false
			l.err = ev.Err
			l.elapsed = ev.Elapsed
		} else {
			l.running = true
		}
		if tty {
			paint()
			return
		}
		if ev.Done {
			status := "ok"
			if ev.Err != nil {
				status = "FAIL"
			}
			fmt.Fprintf(out, "%-14s %s  %s %s\n", ev.Phase, status, l.detail, l.elapsed.Round(10*time.Millisecond))
		}
	})
}
