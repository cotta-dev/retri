package executor

import (
	"errors"
	"testing"

	"github.com/cotta-dev/retri/internal/logger"
)

var errSessionLogWrite = errors.New("session log write failed")

type failingLogWriter struct{}

func (failingLogWriter) Write([]byte) (int, error) { return 0, errSessionLogWrite }

func TestWriteRecordedOutputPropagatesLogFailure(t *testing.T) {
	lg := logger.NewLineLogger(failingLogWriter{}, false)
	if err := writeRecordedOutput(lg, nil, []byte("output\n")); !errors.Is(err, errSessionLogWrite) {
		t.Fatalf("writeRecordedOutput() error = %v, want %v", err, errSessionLogWrite)
	}
}

func TestWriteRecordedOutputPropagatesCommandsOnlyLogFailure(t *testing.T) {
	lg := logger.NewLineLogger(failingLogWriter{}, false)
	recorder := NewCommandOutputRecorder(lg)
	recorder.ObserveOutput([]byte("host$ "))
	recorder.ObserveInput([]byte("echo ok\r"))

	err := writeRecordedOutput(lg, recorder, []byte("echo ok\r\nok\r\nhost$ "))
	if !errors.Is(err, errSessionLogWrite) {
		t.Fatalf("writeRecordedOutput() error = %v, want %v", err, errSessionLogWrite)
	}
}

func TestFlushRecordedOutputPropagatesBufferedLogFailure(t *testing.T) {
	lg := logger.NewLineLogger(failingLogWriter{}, false)
	if _, err := lg.Write([]byte("unterminated")); err != nil {
		t.Fatalf("buffered Write() error = %v", err)
	}
	if err := flushRecordedOutput(lg, nil); !errors.Is(err, errSessionLogWrite) {
		t.Fatalf("flushRecordedOutput() error = %v, want %v", err, errSessionLogWrite)
	}
}
