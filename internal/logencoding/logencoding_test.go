package logencoding

import (
	"bytes"
	"errors"
	"testing"
)

func TestLookupAliases(t *testing.T) {
	tests := map[string]string{
		"":            "raw",
		"binary":      "raw",
		"UTF8":        "utf-8",
		"Windows-31J": "shift_jis",
		"cp932":       "shift_jis",
		"EUC_JP":      "euc-jp",
		"latin1":      "iso-8859-1",
		"cp1252":      "windows-1252",
		"cp936":       "gbk",
		"Big-5":       "big5",
		"EUC KR":      "euc-kr",
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			codec, err := Lookup(input)
			if err != nil {
				t.Fatal(err)
			}
			if got := codec.Name(); got != want {
				t.Fatalf("Name() = %q, want %q", got, want)
			}
		})
	}
}

func TestLookupRejectsUnsupportedEncoding(t *testing.T) {
	if _, err := Lookup("guess"); err == nil {
		t.Fatal("Lookup() accepted an unsupported encoding")
	}
}

func TestDecodeSupportedEncodings(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		want string
	}{
		{name: "utf-8", src: []byte("日本語"), want: "日本語"},
		{name: "shift_jis", src: []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}, want: "日本語"},
		{name: "euc-jp", src: []byte{0xc6, 0xfc, 0xcb, 0xdc, 0xb8, 0xec}, want: "日本語"},
		{name: "iso-8859-1", src: []byte{'c', 'a', 'f', 0xe9}, want: "café"},
		{name: "windows-1252", src: []byte{0x80, '1', '0'}, want: "€10"},
		{name: "gb18030", src: []byte{0xd6, 0xd0, 0xce, 0xc4}, want: "中文"},
		{name: "gbk", src: []byte{0xd6, 0xd0, 0xce, 0xc4}, want: "中文"},
		{name: "big5", src: []byte{0xa4, 0xa4, 0xa4, 0xe5}, want: "中文"},
		{name: "euc-kr", src: []byte{0xc7, 0xd1, 0xb1, 0xdb}, want: "한글"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec, err := Lookup(tt.name)
			if err != nil {
				t.Fatal(err)
			}
			got, err := codec.Decode(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("Decode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeRejectsMalformedInputWithoutReplacement(t *testing.T) {
	codec, err := Lookup("shift_jis")
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := codec.Decode([]byte{0x82})
	if err == nil {
		t.Fatalf("Decode() = %x, want an error", decoded)
	}
	if !errors.Is(err, ErrInvalidSequence) {
		t.Fatalf("Decode() error = %v, want ErrInvalidSequence", err)
	}
	if bytes.Contains(decoded, []byte("�")) {
		t.Fatalf("Decode() returned replacement text: %x", decoded)
	}
}

func TestRawPreservesOriginalSlice(t *testing.T) {
	codec, err := Lookup("raw")
	if err != nil {
		t.Fatal(err)
	}
	src := []byte{0xff, 0x00, 0x80}
	got, err := codec.Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatalf("Decode() = %x, want %x", got, src)
	}
}
