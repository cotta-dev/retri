package logger

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// LineLogger buffers output line-by-line, adding timestamps and stripping ANSI codes.
type LineLogger struct {
	w           io.Writer
	mu          sync.Mutex
	buf         bytes.Buffer
	enabled     bool
	lastIsEmpty bool
}

// NewLineLogger creates a new LineLogger that writes to w.
// If enabled is true, each line is prefixed with a millisecond-precision timestamp.
func NewLineLogger(w io.Writer, enabled bool) *LineLogger {
	return &LineLogger{w: w, enabled: enabled}
}

// Write implements io.Writer. It buffers data and flushes complete lines.
func (l *LineLogger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf.Write(p)

	for {
		data := l.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			break
		}

		line := make([]byte, idx+1)
		_, _ = l.buf.Read(line)
		l.processAndWriteLine(line)
	}
	return len(p), nil
}

// ProcessAndWriteLine processes a raw line (strips ANSI, adds timestamp) and writes it.
func (l *LineLogger) ProcessAndWriteLine(rawLine []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.processAndWriteLine(rawLine)
}

func (l *LineLogger) processAndWriteLine(rawLine []byte) {
	s := string(bytes.ReplaceAll(rawLine, []byte("\r"), nil))
	s = StripAnsi(s)
	s = strings.TrimRight(s, "\n\t ")

	ts := ""
	if l.enabled {
		ts = time.Now().Format("[2006-01-02 15:04:05.000] ")
	}

	if s == "" {
		if l.lastIsEmpty {
			return
		}
		l.lastIsEmpty = true
		_, _ = fmt.Fprintf(l.w, "%s\n", ts)
		return
	}

	l.lastIsEmpty = false
	_, _ = fmt.Fprintf(l.w, "%s%s\n", ts, s)
}

// Flush writes any remaining buffered data (incomplete line without trailing newline).
func (l *LineLogger) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buf.Len() > 0 {
		remaining := l.buf.Bytes()
		l.buf.Reset()
		l.processAndWriteLine(append(remaining, '\n'))
	}
}

// WriteRaw writes content directly without timestamp or processing.
func (l *LineLogger) WriteRaw(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprint(l.w, s)
	l.lastIsEmpty = strings.HasSuffix(s, "\n\n")
}

// LogHeader writes a command header block to the log.
func (l *LineLogger) LogHeader(cmd string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.lastIsEmpty {
		_, _ = l.w.Write([]byte("\n"))
	}

	separator := strings.Repeat("-", 40)
	_, _ = fmt.Fprintf(l.w, "%s\n[EXEC] %s\n%s\n", separator, cmd, separator)
	l.lastIsEmpty = false
}
