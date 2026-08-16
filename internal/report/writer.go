package report

import (
	"fmt"
	"io"
)

// lineWriter keeps the first write error and stops writing after it. A report
// is one message: half of it delivered on a broken pipe or a full disk is worse
// than none, and the caller has to be able to say the report did not come out.
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
