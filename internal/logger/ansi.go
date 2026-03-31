package logger

import "regexp"

// ANSI escape sequence pattern (ported from acarl005/stripansi)
const ansiPattern = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var ansiRe = regexp.MustCompile(ansiPattern)

// controlCharRe matches non-printable control characters except \t (0x09) and \n (0x0A).
// This covers BEL (0x07), BS (0x08), and other control chars that PTY sessions emit.
var controlCharRe = regexp.MustCompile(`[\x00-\x06\x07\x08\x0b\x0c\x0e-\x1f\x7f]`)

// StripAnsi removes ANSI escape sequences and non-printable control characters from a string.
func StripAnsi(str string) string {
	str = ansiRe.ReplaceAllString(str, "")
	str = controlCharRe.ReplaceAllString(str, "")
	return str
}
