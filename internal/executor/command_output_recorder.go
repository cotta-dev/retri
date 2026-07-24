package executor

import (
	"bytes"
	"crypto/sha256"
	"regexp"
	"strings"
	"sync"

	"github.com/cotta-dev/retri/internal/logger"
)

var (
	nonCommandPromptRe = regexp.MustCompile(`(?i)(?:password|passphrase)(?: for [^:?\r\n]*)?[:?]\s*$|are you sure you want to continue connecting[^?]*\?\s*$`)
	pagerPromptRe      = regexp.MustCompile(`(?i)^(?:-{2,}\s*\(?more(?:\s+\d+%)?\)?\s*-{2,}.*|\(end\)(?:\s.*)?|press\s+(?:any\s+key|space(?:bar)?|enter|return)\b.*)$`)
)

const (
	maxBufferedCommandLines = 128
	maxBufferedCommandBytes = 1 << 20
	maxPendingCommandCycles = 1024
)

// CommandOutputRecorder keeps the submitted terminal line and the output that
// follows it while discarding login banners, idle prompts, input redraws, and
// credential prompts. Empty submissions are retained as prompt-only lines. It
// observes the rendered PTY output, so shell-specific hooks and reconstruction
// of individual key bindings are not required.
type CommandOutputRecorder struct {
	mu       sync.Mutex
	target   *logger.LineLogger
	renderer *logger.LineLogger
	gate     *commandOutputGate

	inputStarted   bool
	inputEscape    []byte
	bracketedPaste bool
}

// NewCommandOutputRecorder creates a command/output-only recorder that writes
// accepted lines to target.
func NewCommandOutputRecorder(target *logger.LineLogger) *CommandOutputRecorder {
	gate := &commandOutputGate{target: target}
	return &CommandOutputRecorder{
		target:   target,
		renderer: logger.NewLineLogger(gate, false),
		gate:     gate,
	}
}

// ObserveOutput renders PTY output continuously. Rendering while output is
// filtered is important because the current prompt is needed when Enter is
// pressed later.
func (r *CommandOutputRecorder) ObserveOutput(p []byte) {
	_, _ = r.renderer.Write(p)
}

// ObserveInput records only submission boundaries; command text always comes
// from the terminal's rendered echo. Password bytes are therefore never stored
// or written to the log.
func (r *CommandOutputRecorder) ObserveInput(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Pager controls such as Space, q, and Enter continue or leave the current
	// command's output. Treating them as a new command cycle would hold all later
	// pages until another Enter and could lose them if the session ended first.
	if !r.inputStarted && r.atPagerPrompt() {
		return
	}

	for _, b := range p {
		r.observeInputByte(b)
	}
}

func (r *CommandOutputRecorder) atPagerPrompt() bool {
	return r.gate.consumePagerInput(r.renderer.CurrentLine())
}

func (r *CommandOutputRecorder) observeInputByte(b byte) {
	if len(r.inputEscape) > 0 {
		r.inputEscape = append(r.inputEscape, b)
		if len(r.inputEscape) > 32 {
			r.inputEscape = nil
			return
		}
		if !inputEscapeComplete(r.inputEscape) {
			return
		}
		seq := string(r.inputEscape)
		r.inputEscape = nil
		switch seq {
		case "\x1b[200~":
			r.bracketedPaste = true
		case "\x1b[201~":
			r.bracketedPaste = false
		}
		return
	}

	switch b {
	case '\x1b':
		r.beginInput()
		r.inputEscape = []byte{b}
	case '\r', '\n':
		r.beginInput()
		if !r.bracketedPaste {
			r.gate.Submit()
			r.resetInput()
		}
	case '\x03': // Ctrl-C: discard the cancelled input line.
		r.beginInput()
		r.gate.Cancel()
		r.resetInput()
	default:
		r.beginInput()
	}
}

func (r *CommandOutputRecorder) beginInput() {
	if r.inputStarted {
		return
	}
	baseline := r.renderer.CurrentLine()
	r.inputStarted = true
	r.gate.Begin(baseline, nonCommandPromptRe.MatchString(strings.TrimSpace(baseline)))
}

func (r *CommandOutputRecorder) resetInput() {
	r.inputStarted = false
	r.inputEscape = nil
	r.bracketedPaste = false
}

// Flush discards an idle unterminated prompt, then flushes accepted output.
func (r *CommandOutputRecorder) Flush() {
	r.mu.Lock()
	r.inputStarted = false
	r.inputEscape = nil
	r.bracketedPaste = false
	r.mu.Unlock()

	r.gate.Finish()
	r.renderer.Flush()
	r.target.Flush()
}

func inputEscapeComplete(seq []byte) bool {
	if len(seq) < 2 {
		return false
	}
	if seq[1] == '[' || seq[1] == 'O' {
		if len(seq) < 3 {
			return false
		}
		last := seq[len(seq)-1]
		return last >= 0x40 && last <= 0x7e
	}
	return true
}

type commandCycle struct {
	baseline      [sha256.Size]byte
	sensitive     bool
	submitted     bool
	cancelled     bool
	buffered      [][]byte
	bufferedBytes int
}

type commandOutputGate struct {
	mu           sync.Mutex
	target       *logger.LineLogger
	buf          bytes.Buffer
	active       bool
	pagerWaiting bool
	cycles       []*commandCycle
}

func (g *commandOutputGate) Begin(baseline string, sensitive bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.cycles) < maxPendingCommandCycles {
		g.cycles = append(g.cycles, &commandCycle{baseline: terminalLineFingerprint(baseline), sensitive: sensitive})
	}
}

func (g *commandOutputGate) Submit() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.cycles) > 0 {
		g.cycles[len(g.cycles)-1].submitted = true
	}
}

func (g *commandOutputGate) Cancel() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.cycles) > 0 {
		g.cycles[len(g.cycles)-1].cancelled = true
		g.cycles[len(g.cycles)-1].submitted = true
	}
}

func (g *commandOutputGate) Finish() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active = false
	g.pagerWaiting = false
	g.cycles = nil
	g.buf.Reset()
}

func (g *commandOutputGate) consumePagerInput(currentLine string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	trimmed := strings.TrimSpace(currentLine)
	if isPagerPromptLine(trimmed, g.active || len(g.cycles) > 0) {
		g.pagerWaiting = false
		return true
	}
	if g.pagerWaiting && trimmed == "" {
		g.pagerWaiting = false
		return true
	}
	if trimmed != "" {
		g.pagerWaiting = false
	}
	return false
}

// Write receives already-rendered lines from LineLogger.
func (g *commandOutputGate) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, _ = g.buf.Write(p)
	for {
		line, err := g.buf.ReadBytes('\n')
		if err != nil {
			_, _ = g.buf.Write(line)
			break
		}
		g.handleLine(line)
	}
	return len(p), g.target.Err()
}

func (g *commandOutputGate) handleLine(line []byte) {
	trimmed := strings.TrimSpace(string(line))
	if isPagerPromptLine(trimmed, g.active || len(g.cycles) > 0) {
		g.pagerWaiting = true
		return
	}
	g.pagerWaiting = false

	if len(g.cycles) > 0 {
		cycle := g.cycles[0]
		if !cycle.submitted {
			g.bufferEditingLine(cycle, line)
			return
		}
		g.cycles[0] = nil
		g.cycles = g.cycles[1:]

		if cycle.cancelled {
			g.active = false
			return
		}
		if cycle.sensitive {
			// Keep the previous command active (for example, sudo output) but
			// discard the credential prompt and any non-echoed secret input.
			return
		}

		if len(cycle.buffered) == 0 && sameTerminalLine(line, cycle.baseline) {
			g.writeAccepted(line)
			g.active = false
			return
		}
		for _, buffered := range cycle.buffered {
			g.writeAccepted(buffered)
		}
		g.writeAccepted(line)
		g.active = true
		return
	}

	if g.active && !nonCommandPromptRe.MatchString(trimmed) {
		g.writeAccepted(line)
	}
}

func (g *commandOutputGate) bufferEditingLine(cycle *commandCycle, line []byte) {
	if len(cycle.buffered) >= maxBufferedCommandLines || cycle.bufferedBytes+len(line) > maxBufferedCommandBytes {
		return
	}
	copyOfLine := append([]byte(nil), line...)
	cycle.buffered = append(cycle.buffered, copyOfLine)
	cycle.bufferedBytes += len(copyOfLine)
}

func (g *commandOutputGate) writeAccepted(line []byte) {
	g.target.ProcessAndWriteLine(bytes.TrimSuffix(line, []byte{'\n'}))
}

func sameTerminalLine(line []byte, baseline [sha256.Size]byte) bool {
	return sha256.Sum256(bytes.TrimSpace(line)) == baseline
}

func terminalLineFingerprint(line string) [sha256.Size]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(line)))
}

func isPagerPromptLine(line string, active bool) bool {
	trimmed := strings.TrimSpace(line)
	return pagerPromptRe.MatchString(trimmed) || (active && trimmed == ":")
}
