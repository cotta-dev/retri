package logger

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cotta-dev/retri/internal/logencoding"
)

// LineLogger renders terminal output line-by-line before writing it. PTY output
// is a stream of terminal editing operations, not plain text: carriage returns,
// backspaces, and ANSI cursor controls can redraw an existing prompt. Keeping a
// small virtual line here prevents those redraws from becoming duplicated or
// truncated text in the log.
type LineLogger struct {
	w        io.Writer
	mu       sync.Mutex
	enabled  bool
	codec    logencoding.Codec
	warn     func(error)
	warned   bool
	writeErr error

	line        []terminalCell
	cursor      int
	savedCursor int
	synthetic   int
	utf8Pending []byte
	escapeState terminalEscapeState
	csi         []byte
	stringEsc   bool

	lastIsEmpty bool
}

// terminalCell keeps the exact bytes received for one display cell. Valid UTF-8
// is grouped into a cell for cursor editing, while invalid or legacy-encoded
// bytes are retained one byte at a time instead of being replaced with U+FFFD.
type terminalCell struct {
	data [utf8.UTFMax]byte
	size uint8
}

var terminalSpaceCell = terminalCell{data: [utf8.UTFMax]byte{' '}, size: 1}

type terminalEscapeState uint8

const (
	escapeNone terminalEscapeState = iota
	escapeStarted
	escapeIntermediate
	escapeCSI
	escapeString
)

// Limit cursor-only edits so an untrusted remote host cannot turn a tiny CSI
// sequence into an enormous allocation. Ordinary text is not truncated.
const maxTerminalEdit = 4096

// NewLineLogger creates a new LineLogger that writes to w.
// If enabled is true, each line is prefixed with a millisecond-precision timestamp.
func NewLineLogger(w io.Writer, enabled bool) *LineLogger {
	return newLineLogger(w, enabled, logencoding.Raw(), nil)
}

// NewLineLoggerWithEncoding creates a LineLogger that converts rendered
// terminal lines from encodingName to UTF-8. If a line cannot be converted,
// its original bytes are written and warn is called once for the session.
func NewLineLoggerWithEncoding(w io.Writer, enabled bool, encodingName string, warn func(error)) (*LineLogger, error) {
	codec, err := logencoding.Lookup(encodingName)
	if err != nil {
		return nil, err
	}
	return newLineLogger(w, enabled, codec, warn), nil
}

func newLineLogger(w io.Writer, enabled bool, codec logencoding.Codec, warn func(error)) *LineLogger {
	return &LineLogger{w: w, enabled: enabled, codec: codec, warn: warn}
}

// Write implements io.Writer. Data can be split at any byte boundary, including
// in the middle of UTF-8 characters or terminal escape sequences.
func (l *LineLogger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.writeErr != nil {
		return 0, l.writeErr
	}
	l.feed(p)
	return len(p), l.writeErr
}

// ProcessAndWriteLine processes a complete raw line and writes it immediately.
func (l *LineLogger) ProcessAndWriteLine(rawLine []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.feed(rawLine)
	if len(rawLine) == 0 || rawLine[len(rawLine)-1] != '\n' {
		l.flushLine()
	}
}

func (l *LineLogger) feed(data []byte) {
	for _, b := range data {
		l.feedByte(b)
	}
}

func (l *LineLogger) feedByte(b byte) {
	if l.escapeState != escapeNone {
		l.feedEscapeByte(b)
		return
	}

	if len(l.utf8Pending) > 0 {
		if b >= utf8.RuneSelf {
			l.utf8Pending = append(l.utf8Pending, b)
			l.flushCompleteRunes()
			return
		}
		l.flushPendingBytes()
	}

	switch b {
	case '\x1b':
		l.escapeState = escapeStarted
	case '\r':
		l.cursor = 0
	case '\n':
		l.flushLine()
	case '\b':
		if l.cursor > 0 {
			l.cursor--
		}
	case '\t':
		n := 8 - l.cursor%8
		for range n {
			l.putByte(' ')
		}
	case '\x00', '\x01', '\x02', '\x03', '\x04', '\x05', '\x06', '\x07', '\x0b', '\x0c',
		'\x0e', '\x0f', '\x10', '\x11', '\x12', '\x13', '\x14', '\x15', '\x16', '\x17', '\x18',
		'\x19', '\x1a', '\x1c', '\x1d', '\x1e', '\x1f', '\x7f':
		// Ignore terminal control characters that have no printable representation.
	default:
		// Raw 0x80-0x9f bytes are deliberately not interpreted as 8-bit C1
		// controls: the same values are printable bytes in legacy encodings such
		// as Shift_JIS. Valid UTF-8 encodings of C1 controls are still recognized
		// by putDecodedRune.
		if b < utf8.RuneSelf {
			l.putByte(b)
			return
		}
		l.utf8Pending = append(l.utf8Pending, b)
		l.flushCompleteRunes()
	}
}

func (l *LineLogger) flushCompleteRunes() {
	for len(l.utf8Pending) > 0 && utf8.FullRune(l.utf8Pending) {
		r, size := utf8.DecodeRune(l.utf8Pending)
		if r == utf8.RuneError && size == 1 {
			l.putBytes(l.utf8Pending[:1])
			l.utf8Pending = l.utf8Pending[1:]
			continue
		}
		l.putDecodedRune(r, l.utf8Pending[:size])
		l.utf8Pending = l.utf8Pending[size:]
	}
}

func (l *LineLogger) flushPendingBytes() {
	for len(l.utf8Pending) > 0 {
		if utf8.FullRune(l.utf8Pending) {
			r, size := utf8.DecodeRune(l.utf8Pending)
			if r != utf8.RuneError || size > 1 {
				l.putDecodedRune(r, l.utf8Pending[:size])
				l.utf8Pending = l.utf8Pending[size:]
				continue
			}
		}
		l.putBytes(l.utf8Pending[:1])
		l.utf8Pending = l.utf8Pending[1:]
	}
}

func (l *LineLogger) putDecodedRune(r rune, raw []byte) {
	switch r {
	case '\u009b':
		l.escapeState = escapeCSI
		l.csi = l.csi[:0]
	case '\u0090', '\u0098', '\u009d', '\u009e', '\u009f':
		l.escapeState = escapeString
		l.stringEsc = false
	default:
		if r < '\u0080' || r > '\u009f' {
			l.putBytes(raw)
		}
	}
}

func (l *LineLogger) ensureCursor() {
	if l.cursor > len(l.line) {
		gap := min(l.cursor-len(l.line), max(maxTerminalEdit-l.synthetic, 0))
		l.cursor = len(l.line) + gap
		for range gap {
			l.line = append(l.line, terminalSpaceCell)
		}
		l.synthetic += gap
	}
}

func (l *LineLogger) putByte(b byte) {
	l.putBytes([]byte{b})
}

func (l *LineLogger) putBytes(raw []byte) {
	l.ensureCursor()
	var cell terminalCell
	cell.size = uint8(copy(cell.data[:], raw))
	if l.cursor == len(l.line) {
		l.line = append(l.line, cell)
	} else {
		l.line[l.cursor] = cell
	}
	l.cursor++
}

func (l *LineLogger) feedEscapeByte(b byte) {
	switch l.escapeState {
	case escapeStarted:
		switch b {
		case '[':
			l.escapeState = escapeCSI
			l.csi = l.csi[:0]
		case ']', 'P', 'X', '^', '_':
			l.escapeState = escapeString
			l.stringEsc = false
		case '\x1b':
			// Stay in escapeStarted for repeated ESC bytes.
		default:
			if b >= 0x20 && b <= 0x2f {
				l.escapeState = escapeIntermediate
				return
			}
			l.handleSingleEscape(b)
			l.escapeState = escapeNone
		}
	case escapeIntermediate:
		if b >= 0x30 && b <= 0x7e {
			l.escapeState = escapeNone
		} else if b == '\x1b' {
			l.escapeState = escapeStarted
		}
	case escapeCSI:
		if b >= 0x40 && b <= 0x7e {
			l.handleCSI(b, string(l.csi))
			l.csi = l.csi[:0]
			l.escapeState = escapeNone
			return
		}
		if b == '\x1b' {
			l.csi = l.csi[:0]
			l.escapeState = escapeStarted
			return
		}
		if len(l.csi) < 128 {
			l.csi = append(l.csi, b)
		}
	case escapeString:
		if b == '\x07' || b == '\x9c' || (l.stringEsc && b == '\\') {
			l.escapeState = escapeNone
			l.stringEsc = false
			return
		}
		l.stringEsc = b == '\x1b'
	}
}

func (l *LineLogger) handleSingleEscape(final byte) {
	switch final {
	case '7':
		l.savedCursor = l.cursor
	case '8':
		l.cursor = l.savedCursor
	}
}

func (l *LineLogger) handleCSI(final byte, rawParams string) {
	params := parseCSIParams(rawParams)
	first := max(1, min(csiParam(params, 0, 1), maxTerminalEdit))
	maxCursor := len(l.line) + maxTerminalEdit

	switch final {
	case 'G', '`':
		l.cursor = min(max(first-1, 0), maxCursor)
	case 'C', 'a':
		l.cursor = min(l.cursor+first, maxCursor)
	case 'D':
		l.cursor = max(l.cursor-first, 0)
	case 'H', 'f':
		column := max(1, min(csiParam(params, 1, 1), maxTerminalEdit))
		l.cursor = min(max(column-1, 0), maxCursor)
	case 'K':
		mode := csiParam(params, 0, 0)
		switch mode {
		case 0:
			if l.cursor < len(l.line) {
				l.line = l.line[:l.cursor]
			}
		case 1:
			end := min(l.cursor+1, len(l.line))
			for i := range end {
				l.line[i] = terminalSpaceCell
			}
		case 2:
			l.line = nil
			l.cursor = 0
			l.synthetic = 0
		}
	case 'P':
		if l.cursor < len(l.line) {
			end := min(l.cursor+first, len(l.line))
			l.line = append(l.line[:l.cursor], l.line[end:]...)
		}
	case '@':
		l.ensureCursor()
		count := min(first, max(maxTerminalEdit-l.synthetic, 0))
		spaces := make([]terminalCell, count)
		for i := range spaces {
			spaces[i] = terminalSpaceCell
		}
		l.line = append(l.line[:l.cursor], append(spaces, l.line[l.cursor:]...)...)
		l.synthetic += count
	case 'X':
		for i := 0; i < first; i++ {
			pos := l.cursor + i
			if pos >= len(l.line) {
				break
			}
			l.line[pos] = terminalSpaceCell
		}
	case 's':
		l.savedCursor = l.cursor
	case 'u':
		l.cursor = l.savedCursor
	}
}

func parseCSIParams(raw string) []int {
	raw = strings.TrimLeft(raw, "?<=>!")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	params := make([]int, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			params[i] = n
		}
	}
	return params
}

func csiParam(params []int, index, defaultValue int) int {
	if index >= len(params) || params[index] == 0 {
		return defaultValue
	}
	return params[index]
}

func (l *LineLogger) flushLine() {
	l.flushPendingBytes()
	s := bytes.TrimRight(l.lineBytes(), "\t ")
	l.line = nil
	l.cursor = 0
	l.savedCursor = 0
	l.synthetic = 0

	ts := ""
	if l.enabled {
		ts = time.Now().Format("[2006-01-02 15:04:05.000] ")
	}

	if len(s) == 0 {
		if l.lastIsEmpty {
			return
		}
		l.lastIsEmpty = true
		l.writeString(ts + "\n")
		return
	}

	l.lastIsEmpty = false
	output, err := l.codec.Decode(s)
	if err != nil {
		output = s
		if !l.warned && l.warn != nil {
			l.warn(err)
		}
		l.warned = true
	}
	l.writeString(ts)
	l.writeBytes(output)
	l.writeBytes([]byte{'\n'})
}

func (l *LineLogger) writeString(s string) {
	if l.writeErr != nil {
		return
	}
	n, err := io.WriteString(l.w, s)
	if err == nil && n != len(s) {
		err = io.ErrShortWrite
	}
	if err != nil {
		l.writeErr = err
	}
}

func (l *LineLogger) writeBytes(p []byte) {
	if l.writeErr != nil {
		return
	}
	n, err := l.w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		l.writeErr = err
	}
}

func (l *LineLogger) lineBytes() []byte {
	size := 0
	for _, cell := range l.line {
		size += int(cell.size)
	}

	line := make([]byte, 0, size)
	for _, cell := range l.line {
		line = append(line, cell.data[:cell.size]...)
	}
	return line
}

func (l *LineLogger) resetEscape() {
	l.escapeState = escapeNone
	l.csi = l.csi[:0]
	l.stringEsc = false
}

func (l *LineLogger) flushPendingLine() {
	l.resetEscape()
	l.flushPendingBytes()
	if len(l.line) > 0 {
		l.flushLine()
	}
}

// Flush writes any remaining visible terminal line without requiring a newline.
func (l *LineLogger) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushPendingLine()
}

// Err returns the first log write error. Errors are sticky so callers can
// check once after a session even when writes happen through io.Writer.
func (l *LineLogger) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writeErr
}

// CurrentLine returns the terminal's currently visible, unterminated line.
// It is intended for PTY coordination such as distinguishing an idle prompt
// from a submitted command; it does not flush or otherwise change logger state.
func (l *LineLogger) CurrentLine() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.lineBytes())
}

// WriteRaw writes content directly without timestamp or processing. Any pending
// terminal line is emitted first so a final prompt cannot appear after a footer.
func (l *LineLogger) WriteRaw(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushPendingLine()
	l.writeString(s)
	l.lastIsEmpty = strings.HasSuffix(s, "\n\n")
}

// LogHeader writes a command header block immediately before the next command
// line. When preservePendingLine is true, a pending prompt remains buffered so
// the terminal's own command echo completes it as "prompt + command". Otherwise,
// unterminated output is flushed before the header.
func (l *LineLogger) LogHeader(cmd string, preservePendingLine bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !preservePendingLine {
		l.flushPendingLine()
	}

	if !l.lastIsEmpty {
		l.writeBytes([]byte("\n"))
	}

	separator := strings.Repeat("-", 40)
	l.writeString(fmt.Sprintf("%s\n[EXEC] %s\n%s\n", separator, cmd, separator))
	l.lastIsEmpty = false
}
