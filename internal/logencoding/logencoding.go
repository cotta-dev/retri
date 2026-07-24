// Package logencoding converts explicitly configured terminal encodings to
// UTF-8 without silently replacing malformed input.
package logencoding

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

var (
	// ErrInvalidSequence means terminal bytes do not form a valid sequence in
	// the configured source encoding.
	ErrInvalidSequence = errors.New("invalid byte sequence")
	nameReplacer       = strings.NewReplacer("-", "_", " ", "_", ".", "_")
	replacementRune    = []byte(string(utf8.RuneError))
)

const supportedNames = "raw, utf-8, shift_jis, euc-jp, iso-8859-1, windows-1252, gb18030, gbk, big5, euc-kr"

// Codec is an immutable description of a supported terminal encoding.
type Codec struct {
	name     string
	encoding encoding.Encoding
	raw      bool
	utf8     bool
}

// Name returns the canonical configuration value for the codec.
func (c Codec) Name() string { return c.name }

// Raw returns the byte-preserving codec used when no conversion is requested.
func Raw() Codec { return Codec{name: "raw", raw: true} }

// Lookup resolves a user-facing encoding name. An empty value deliberately
// means raw so existing configurations continue to preserve terminal bytes.
func Lookup(name string) (Codec, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = nameReplacer.Replace(normalized)

	switch normalized {
	case "", "raw", "none", "binary":
		return Raw(), nil
	case "utf8", "utf_8":
		return Codec{name: "utf-8", utf8: true}, nil
	case "shift_jis", "shiftjis", "sjis", "cp932", "windows_31j", "ms_kanji":
		return Codec{name: "shift_jis", encoding: japanese.ShiftJIS}, nil
	case "euc_jp", "eucjp":
		return Codec{name: "euc-jp", encoding: japanese.EUCJP}, nil
	case "iso_8859_1", "iso8859_1", "latin1", "latin_1":
		return Codec{name: "iso-8859-1", encoding: charmap.ISO8859_1}, nil
	case "windows_1252", "cp1252":
		return Codec{name: "windows-1252", encoding: charmap.Windows1252}, nil
	case "gb18030":
		return Codec{name: "gb18030", encoding: simplifiedchinese.GB18030}, nil
	case "gbk", "cp936":
		return Codec{name: "gbk", encoding: simplifiedchinese.GBK}, nil
	case "big5", "big_5":
		return Codec{name: "big5", encoding: traditionalchinese.Big5}, nil
	case "euc_kr", "euckr", "ks_c_5601_1987":
		return Codec{name: "euc-kr", encoding: korean.EUCKR}, nil
	default:
		return Codec{}, fmt.Errorf("unsupported log_encoding %q (supported: %s)", name, supportedNames)
	}
}

// Decode converts one rendered terminal line to UTF-8. It treats the
// replacement rune emitted by x/text for malformed source bytes as a failure,
// unless it round-trips to the exact original byte sequence.
func (c Codec) Decode(src []byte) ([]byte, error) {
	if c.raw {
		return src, nil
	}
	if c.utf8 {
		if !utf8.Valid(src) {
			return nil, fmt.Errorf("%w for %s", ErrInvalidSequence, c.name)
		}
		return src, nil
	}

	decoded, err := c.encoding.NewDecoder().Bytes(src)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", c.name, err)
	}
	if bytes.Contains(decoded, replacementRune) {
		roundTrip, encodeErr := c.encoding.NewEncoder().Bytes(decoded)
		if encodeErr != nil || !bytes.Equal(roundTrip, src) {
			return nil, fmt.Errorf("%w for %s", ErrInvalidSequence, c.name)
		}
	}
	return decoded, nil
}
