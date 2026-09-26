package logger

import (
	"bytes"
	"io"
)

// SecretWriter suppresses known literal secrets across Write boundaries. It
// cannot detect arbitrary encodings or transformations performed by a peer.
type SecretWriter struct {
	w       io.Writer
	secrets []string
	pending []byte
}

func NewSecretWriter(w io.Writer, secrets ...string) *SecretWriter {
	r := &SecretWriter{w: w}
	for _, secret := range secrets {
		if secret != "" {
			r.secrets = append(r.secrets, secret)
		}
	}
	return r
}

func (r *SecretWriter) Write(p []byte) (int, error) {
	r.pending = append(r.pending, p...)
	if err := r.drain(false); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (r *SecretWriter) Flush() error { return r.drain(true) }

func (r *SecretWriter) drain(final bool) error {
	var output []byte
	for len(r.pending) > 0 {
		match, partial := 0, false
		for _, secret := range r.secrets {
			if len(r.pending) < len(secret) && bytes.HasPrefix([]byte(secret), r.pending) {
				partial = true
			}
			if len(secret) <= len(r.pending) && bytes.HasPrefix(r.pending, []byte(secret)) && len(secret) > match {
				match = len(secret)
			}
		}
		if partial && !final {
			break
		}
		if partial && final {
			match = len(r.pending)
		}
		if match > 0 {
			output = append(output, "[REDACTED]"...)
			clear(r.pending[:match])
			r.pending = r.pending[match:]
		} else {
			output = append(output, r.pending[0])
			r.pending[0] = 0
			r.pending = r.pending[1:]
		}
	}
	if len(output) == 0 {
		return nil
	}
	n, err := r.w.Write(output)
	if err == nil && n != len(output) {
		err = io.ErrShortWrite
	}
	return err
}

// RedactSecrets must be set before any log output is written.
func (l *LineLogger) RedactSecrets(secrets ...string) { l.w = NewSecretWriter(l.w, secrets...) }
