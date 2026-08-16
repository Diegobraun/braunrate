package report

import (
	"fmt"
	"io"
)

// Keeps the first write error and stops: half a report on a broken pipe is
// worse than none, and the caller has to be able to say it did not come out.
type lineWriter struct {
	out io.Writer
	err error
}

func (w *lineWriter) writef(format string, args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.out, format+"\n", args...)
}
