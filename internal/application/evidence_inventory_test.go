package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/session"
)

type evidenceInventoryTestStore struct {
	run         domain.Run
	mission     domain.Mission
	attachments []session.EvidenceAttachment
}

func (s *evidenceInventoryTestStore) GetRun(_ context.Context,
	id string,
) (domain.Run, error) {
	if id != s.run.ID {
		return domain.Run{}, errors.New("Run not found")
	}
	return s.run, nil
}

func (s *evidenceInventoryTestStore) GetMission(_ context.Context,
	id string,
) (domain.Mission, error) {
	if id != s.mission.ID {
		return domain.Mission{}, errors.New("Mission not found")
	}
	return s.mission, nil
}

func (s *evidenceInventoryTestStore) ListEvidenceAttachments(_ context.Context,
	_ string, _ int,
) ([]session.EvidenceAttachment, error) {
	return append([]session.EvidenceAttachment(nil), s.attachments...), nil
}

func TestEvidenceInventoryOmitsMessageAndOperationAuthority(t *testing.T) {
	now := time.Now().UTC()
	privateKey := strings.Repeat("a", 64)
	state := &evidenceInventoryTestStore{
		run:     domain.Run{ID: "run-evidence", MissionID: "mission-evidence", SessionID: "session-evidence"},
		mission: domain.Mission{ID: "mission-evidence", WorkspaceID: "workspace-evidence"},
		attachments: []session.EvidenceAttachment{{ID: "evidence-one",
			ProtocolVersion:    session.EvidenceAttachmentProtocolVersion,
			OperationKeyDigest: privateKey, RequestFingerprint: strings.Repeat("b", 64),
			RunID: "run-evidence", SessionID: "session-evidence", WorkspaceID: "workspace-evidence",
			SourceKind: session.SourceWorkspaceFile, SourceRef: "README.md",
			ContentSHA256: strings.Repeat("c", 64), SessionMessageID: 7,
			AttachedBy: "PRIVATE-operator", EventSequence: 9, CreatedAt: now}},
	}
	inventory, err := NewEvidenceInventoryService(state).List(t.Context(), state.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.ProtocolVersion != session.EvidenceInventoryProtocolVersion ||
		inventory.Truncated || len(inventory.Items) != 1 ||
		inventory.Items[0].InstructionAuthorized || inventory.Items[0].SourceRef != "README.md" {
		t.Fatalf("unexpected evidence inventory: %#v", inventory)
	}
	if _, err := NewEvidenceInventoryService(state).List(t.Context(), " run-evidence"); err == nil {
		t.Fatal("non-normalized evidence Run identity was accepted")
	}
	state.attachments[0].SessionID = "session-other"
	if _, err := NewEvidenceInventoryService(state).List(t.Context(), state.run.ID); err == nil {
		t.Fatal("cross-Session evidence attachment was accepted")
	}
}
