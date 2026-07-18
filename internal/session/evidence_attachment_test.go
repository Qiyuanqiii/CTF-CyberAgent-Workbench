package session

import (
	"strings"
	"testing"
	"time"
)

func TestEvidenceAttachmentRequiresCanonicalWorkspaceFileReference(t *testing.T) {
	baseline := EvidenceAttachment{
		ID: "evidence-1", ProtocolVersion: EvidenceAttachmentProtocolVersion,
		OperationKeyDigest: strings.Repeat("a", 64),
		RequestFingerprint: strings.Repeat("b", 64),
		RunID:              "run-1", SessionID: "session-1", WorkspaceID: "workspace-1",
		SourceKind: SourceWorkspaceFile, SourceRef: "docs/README.md",
		ContentSHA256: strings.Repeat("c", 64), SessionMessageID: 1,
		AttachedBy: "operator", EventSequence: 1, CreatedAt: time.Now().UTC(),
	}
	if err := baseline.Validate(); err != nil {
		t.Fatalf("canonical evidence reference was rejected: %v", err)
	}
	for _, sourceRef := range []string{".", "../README.md", "docs/../README.md",
		"docs/./README.md", "/README.md", `C:\README.md`, `docs\README.md`,
		"docs//README.md"} {
		value := baseline
		value.SourceRef = sourceRef
		if err := value.Validate(); err == nil {
			t.Fatalf("non-canonical evidence reference %q was accepted", sourceRef)
		}
	}
}
