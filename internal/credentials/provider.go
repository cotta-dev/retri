package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cotta-dev/retri/internal/config"
	"golang.org/x/term"
)

func (r *Resolver) resolveProvider(name string, spec config.CredentialSpec) (string, error) {
	switch strings.ToLower(strings.TrimSpace(spec.Provider)) {
	case "prompt":
		label := strings.TrimSpace(spec.Prompt)
		if label == "" {
			label = fmt.Sprintf("Credential %q", name)
		}
		r.promptMu.Lock()
		defer r.promptMu.Unlock()
		value, err := r.prompt(label)
		if err != nil {
			return "", fmt.Errorf("credential %q: prompt failed: %w", name, err)
		}
		return value, nil
	case "env":
		value, ok := os.LookupEnv(spec.Ref)
		if !ok {
			return "", fmt.Errorf("credential %q: environment variable %q is not set", name, spec.Ref)
		}
		return value, nil
	case "literal":
		return os.ExpandEnv(spec.Value), nil
	case "keyring":
		value, found, err := readSourceKey(spec.Ref)
		if err != nil {
			return "", fmt.Errorf("credential %q: keyring: %w", name, err)
		}
		if !found {
			return "", fmt.Errorf("credential %q: keyring entry was not found", name)
		}
		return value, nil
	case "bitwarden":
		value, err := r.readBitwarden(spec)
		if err != nil {
			return "", fmt.Errorf("credential %q: %w", name, err)
		}
		return value, nil
	default:
		return "", fmt.Errorf("credential %q: unsupported provider", name)
	}
}

func readHiddenPrompt(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("prompt requires a terminal on stdin")
	}
	fmt.Fprintf(os.Stderr, "%s: ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	defer clear(b)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *Resolver) readBitwarden(spec config.CredentialSpec) (string, error) {
	if strings.TrimSpace(os.Getenv("BW_SESSION")) == "" {
		return "", fmt.Errorf("BW_SESSION is not set; run `export BW_SESSION=\"$(bw unlock --raw)\"` in the same shell before starting retri")
	}
	if spec.Server != "" {
		out, err := r.run("bw", []string{"status", "--nointeraction"}, nil)
		if err != nil {
			return "", fmt.Errorf("bw status failed; log in and export BW_SESSION: %w", err)
		}
		var status struct {
			ServerURL string `json:"serverUrl"`
			UserID    string `json:"userId"`
			Status    string `json:"status"`
		}
		err = json.Unmarshal(out, &status)
		clear(out)
		if err != nil {
			return "", fmt.Errorf("invalid bw status response")
		}
		if strings.TrimRight(status.ServerURL, "/") != strings.TrimRight(spec.Server, "/") || status.UserID != spec.Account {
			return "", fmt.Errorf("bw profile does not match configured server/account")
		}
		if status.Status != "unlocked" {
			return "", fmt.Errorf("bw vault is not unlocked; export BW_SESSION from bw unlock")
		}
	}
	out, err := r.run("bw", []string{"get", "item", spec.Ref, "--nointeraction"}, nil)
	defer clear(out)
	if err != nil {
		return "", fmt.Errorf("bw get item failed; check login, BW_SESSION and item access: %w", err)
	}
	var item struct {
		Login *struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"login"`
		Fields []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(out, &item); err != nil {
		return "", fmt.Errorf("invalid bw item response")
	}
	field := strings.TrimSpace(spec.Field)
	if field == "" {
		field = "password"
	}
	switch strings.ToLower(field) {
	case "password", "username":
		if item.Login == nil {
			return "", fmt.Errorf("bitwarden item has no login")
		}
		if strings.EqualFold(field, "username") {
			return item.Login.Username, nil
		}
		return item.Login.Password, nil
	default:
		for _, f := range item.Fields {
			if f.Name == field {
				return f.Value, nil
			}
		}
		return "", fmt.Errorf("bitwarden item field was not found")
	}
}
