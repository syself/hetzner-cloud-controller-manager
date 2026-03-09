package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadHcloudCredentials(t *testing.T) {
	t.Run("reads hcloud key", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "hcloud"), []byte(" token-from-hcloud \n"), 0o600)
		if err != nil {
			t.Fatalf("write hcloud token: %v", err)
		}

		token, err := readHcloudCredentials(dir)
		if err != nil {
			t.Fatalf("readHcloudCredentials() error = %v", err)
		}
		if token != "token-from-hcloud" {
			t.Fatalf("readHcloudCredentials() token = %q, want %q", token, "token-from-hcloud")
		}
	})

	t.Run("falls back to token key", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "token"), []byte("token-from-token-key"), 0o600)
		if err != nil {
			t.Fatalf("write token key: %v", err)
		}

		token, err := readHcloudCredentials(dir)
		if err != nil {
			t.Fatalf("readHcloudCredentials() error = %v", err)
		}
		if token != "token-from-token-key" {
			t.Fatalf("readHcloudCredentials() token = %q, want %q", token, "token-from-token-key")
		}
	})

	t.Run("prefers hcloud key over token key", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "hcloud"), []byte("first-token"), 0o600)
		if err != nil {
			t.Fatalf("write hcloud token: %v", err)
		}
		err = os.WriteFile(filepath.Join(dir, "token"), []byte("second-token"), 0o600)
		if err != nil {
			t.Fatalf("write token key: %v", err)
		}

		token, err := readHcloudCredentials(dir)
		if err != nil {
			t.Fatalf("readHcloudCredentials() error = %v", err)
		}
		if token != "first-token" {
			t.Fatalf("readHcloudCredentials() token = %q, want %q", token, "first-token")
		}
	})

	t.Run("returns error if no key exists", func(t *testing.T) {
		dir := t.TempDir()

		_, err := readHcloudCredentials(dir)
		if err == nil {
			t.Fatal("readHcloudCredentials() error = nil, want non-nil")
		}

		errText := err.Error()
		if !strings.Contains(errText, filepath.Join(dir, "hcloud")) {
			t.Fatalf("error does not mention hcloud key path: %q", errText)
		}
		if !strings.Contains(errText, filepath.Join(dir, "token")) {
			t.Fatalf("error does not mention token key path: %q", errText)
		}
	})
}
