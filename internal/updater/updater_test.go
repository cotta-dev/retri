package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestValidateReleaseDownloadURL(t *testing.T) {
	valid := "https://github.com/cotta-dev/retri/releases/download/v1.2.3/retri_1.2.3_amd64.deb"
	if err := validateReleaseDownloadURL(valid); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}

	for _, invalid := range []string{
		"http://github.com/cotta-dev/retri/releases/download/v1.2.3/retri.deb",
		"https://example.com/cotta-dev/retri/releases/download/v1.2.3/retri.deb",
		"https://github.com/other/repo/releases/download/v1.2.3/retri.deb",
	} {
		if err := validateReleaseDownloadURL(invalid); err == nil {
			t.Fatalf("untrusted URL accepted: %s", invalid)
		}
	}
}

func TestDownloadValidatesResponseAndCopiesPackage(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Status:        "200 OK",
				Body:          io.NopCloser(bytes.NewBufferString("package")),
				ContentLength: int64(len("package")),
				Header:        make(http.Header),
				Request:       req,
			}, nil
		}),
	}
	var dst bytes.Buffer
	if err := download(context.Background(), client, "https://example.invalid/package.deb", &dst); err != nil {
		t.Fatal(err)
	}
	if got, want := dst.String(), "package"; got != want {
		t.Fatalf("downloaded content = %q, want %q", got, want)
	}
}

func TestDownloadRejectsErrorStatusAndOversizedPackage(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		contentLength   int64
		responseContent string
	}{
		{name: "error status", status: http.StatusNotFound, contentLength: 0},
		{name: "oversized content length", status: http.StatusOK, contentLength: maxUpdateSize + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode:    tt.status,
						Status:        http.StatusText(tt.status),
						Body:          io.NopCloser(bytes.NewBufferString(tt.responseContent)),
						ContentLength: tt.contentLength,
						Header:        make(http.Header),
						Request:       req,
					}, nil
				}),
			}
			if err := download(context.Background(), client, "https://example.invalid/package.deb", io.Discard); err == nil {
				t.Fatal("download() accepted an invalid response")
			}
		})
	}
}
