package progress

import "testing"

func TestReporterCloseIsIdempotent(t *testing.T) {
	closed := 0
	r := NewWithClose(nil, func() { closed++ })
	r.Close()
	r.Close()
	if closed != 1 {
		t.Fatalf("close callback ran %d times", closed)
	}
}
