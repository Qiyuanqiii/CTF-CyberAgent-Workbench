package artifact

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeCaptureRequestRedactsAndCopiesOutputs(t *testing.T) {
	token := "s" + "k-" + strings.Repeat("a", 28)
	outputs := []Output{{Stream: StreamStdout, MIME: " text/plain; charset=utf-8 ", Content: "token=" + token}}
	normalized, err := NormalizeCaptureRequest(CaptureRequest{
		RunID: "run-1", SessionID: "sess-1", WorkspaceID: "ws-1", SourceID: "tool-1",
		ToolName: "shell", Outputs: outputs,
	})
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].Content = "changed"
	if normalized.Outputs[0].MIME != "text/plain; charset=utf-8" ||
		strings.Contains(normalized.Outputs[0].Content, token) || !normalized.Outputs[0].Redacted ||
		!strings.Contains(normalized.Outputs[0].Content, "[REDACTED:") {
		t.Fatalf("capture request was not safely normalized: %#v", normalized)
	}
}

func TestNormalizeCaptureRequestRejectsInvalidOutputs(t *testing.T) {
	base := CaptureRequest{
		RunID: "run-1", SessionID: "sess-1", SourceID: "tool-1", ToolName: "shell",
		Outputs: []Output{{Stream: StreamStdout, MIME: "text/plain", Content: "ok"}},
	}
	tests := []struct {
		name   string
		mutate func(*CaptureRequest)
	}{
		{name: "missing run", mutate: func(request *CaptureRequest) { request.RunID = "" }},
		{name: "duplicate stream", mutate: func(request *CaptureRequest) {
			request.Outputs = append(request.Outputs, request.Outputs[0])
		}},
		{name: "invalid stream", mutate: func(request *CaptureRequest) { request.Outputs[0].Stream = "combined" }},
		{name: "invalid mime", mutate: func(request *CaptureRequest) { request.Outputs[0].MIME = "not a mime" }},
		{name: "invalid utf8", mutate: func(request *CaptureRequest) { request.Outputs[0].Content = string([]byte{0xff}) }},
		{name: "oversized", mutate: func(request *CaptureRequest) {
			request.Outputs[0].Content = strings.Repeat("x", MaxContentBytes+1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			request.Outputs = append([]Output(nil), base.Outputs...)
			test.mutate(&request)
			if _, err := NormalizeCaptureRequest(request); err == nil {
				t.Fatal("expected artifact request rejection")
			}
		})
	}
}

func TestBlobValidationDetectsContentTampering(t *testing.T) {
	content := "durable output"
	blob := Blob{Descriptor: Descriptor{
		ID: "artifact-1", RunID: "run-1", SessionID: "sess-1", SourceID: "tool-1", ToolName: "shell",
		Stream: StreamStdout, Kind: KindToolOutput, MIME: "text/plain; charset=utf-8", Encoding: EncodingUTF8,
		SHA256: Hash(content), SizeBytes: int64(len(content)), CreatedAt: time.Now().UTC(),
	}, Content: content}
	if err := blob.Validate(); err != nil {
		t.Fatal(err)
	}
	blob.Content = "tampered output"
	if err := blob.Validate(); err == nil {
		t.Fatal("expected artifact integrity failure")
	}
}
