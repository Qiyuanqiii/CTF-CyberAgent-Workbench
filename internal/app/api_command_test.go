package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *synchronizedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(value)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func TestAPIServeCLIStartsAuthenticatedLoopbackServerWithoutPersistingToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	t.Setenv("MIMO_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("CYBERAGENT_ANTHROPIC_API_KEY", "")
	token := "cli-api-token-0123456789-abcdefghijkl"
	t.Setenv(apiTokenEnvironment, token)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var stdout synchronizedBuffer
	var stderr synchronizedBuffer
	done := make(chan int, 1)
	go func() {
		done <- ExecuteContext(ctx, []string{"api", "serve", "--listen", "127.0.0.1:0"}, &stdout, &stderr)
	}()

	var baseURL string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		baseURL = outputField(stdout.String(), "api_url")
		if baseURL != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if baseURL == "" {
		t.Fatalf("API did not report its URL: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), token) ||
		!strings.Contains(stdout.String(), "api_token_source: "+apiTokenEnvironment) ||
		!strings.Contains(stdout.String(), "api_token_generated: false") {
		t.Fatalf("environment token reporting is unsafe or incomplete: %s", stdout.String())
	}

	request, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || !json.Valid(body) ||
		!bytes.Contains(body, []byte(`"version":"api.v1"`)) {
		t.Fatalf("unexpected CLI API response: status=%d body=%s err=%v", response.StatusCode, body, readErr)
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("API command exited with code %d: %s", code, stderr.String())
		}
	case <-time.After(4 * time.Second):
		t.Fatal("API command did not stop after context cancellation")
	}
	if stderr.String() != "" {
		t.Fatalf("API command wrote unexpected stderr: %s", stderr.String())
	}
	assertDirectoryOmitsValue(t, home, token)
}

func TestAPIServeCLIGeneratesUsableProcessToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	t.Setenv(apiTokenEnvironment, "")
	t.Setenv("MIMO_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("CYBERAGENT_ANTHROPIC_API_KEY", "")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var stdout synchronizedBuffer
	var stderr synchronizedBuffer
	done := make(chan int, 1)
	go func() {
		done <- ExecuteContext(ctx, []string{"api", "serve", "--listen", "127.0.0.1:0"}, &stdout, &stderr)
	}()

	var baseURL string
	var token string
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		output := stdout.String()
		baseURL = outputField(output, "api_url")
		token = outputField(output, "api_token")
		if baseURL != "" && token != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if baseURL == "" || len(token) < 32 || !strings.Contains(stdout.String(), "api_token_generated: true") {
		t.Fatalf("generated API credentials are incomplete: %s", stdout.String())
	}
	request, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("generated token was unusable: status=%d err=%v", response.StatusCode, readErr)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 || stderr.String() != "" {
			t.Fatalf("generated-token API exit failed: code=%d stderr=%s", code, stderr.String())
		}
	case <-time.After(4 * time.Second):
		t.Fatal("generated-token API did not stop")
	}
	assertDirectoryOmitsValue(t, home, token)
}

func TestAPIServeCLIRejectsUnsafeConfiguration(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	tests := []struct {
		name  string
		token string
		args  []string
		code  int
		text  string
	}{
		{name: "short token", token: "short", args: []string{"api", "serve", "--listen", "127.0.0.1:0"},
			code: 2, text: "access token"},
		{name: "non-normalized token", token: strings.Repeat("x", 32) + " ",
			args: []string{"api", "serve", "--listen", "127.0.0.1:0"}, code: 2, text: "access token"},
		{name: "public bind", token: strings.Repeat("x", 32),
			args: []string{"api", "serve", "--listen", "0.0.0.0:0"}, code: 5, text: "loopback"},
		{name: "unknown subcommand", token: strings.Repeat("x", 32),
			args: []string{"api", "publish"}, code: 2, text: "unknown API subcommand"},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			t.Setenv(apiTokenEnvironment, current.token)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := ExecuteContext(context.Background(), current.args, &stdout, &stderr)
			if code != current.code || !strings.Contains(stderr.String(), current.text) || stdout.Len() != 0 {
				t.Fatalf("unexpected CLI result: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
		})
	}
}

func outputField(output string, key string) string {
	prefix := key + ": "
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func assertDirectoryOmitsValue(t *testing.T, root string, forbidden string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, []byte(forbidden)) {
			t.Errorf("API token was persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
