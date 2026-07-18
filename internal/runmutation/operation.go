package runmutation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxIdentityRunes = 256

type TargetKind string

const (
	TargetWorkItem TargetKind = "work_item"
	TargetNote     TargetKind = "note"
)

func (k TargetKind) Valid() bool {
	return k == TargetWorkItem || k == TargetNote
}

type Operation struct {
	KeyDigest          string
	RequestFingerprint string
	InvocationID       string
	RunID              string
	SessionID          string
	WorkspaceID        string
	LeaseID            string
	LeaseGeneration    int64
	ToolName           string
	TargetKind         TargetKind
	TargetID           string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o Operation) Validate() error {
	return o.validate(true)
}

// ValidateStored accepts records without an execution lease because leases are
// transient fencing credentials and are intentionally not persisted with intent.
func (o Operation) ValidateStored() error {
	return o.validate(false)
}

func (o Operation) validate(requireSupervisorLease bool) error {
	for label, value := range map[string]string{
		"invocation id": o.InvocationID, "run id": o.RunID, "session id": o.SessionID,
		"workspace id": o.WorkspaceID, "tool name": o.ToolName, "target id": o.TargetID,
		"lease id": o.LeaseID, "requester": o.RequestedBy,
	} {
		if !utf8.ValidString(value) || strings.TrimSpace(value) != value || len([]rune(value)) > MaxIdentityRunes {
			return fmt.Errorf("structured mutation %s must be normalized and bounded UTF-8", label)
		}
	}
	if o.InvocationID == "" || o.RunID == "" || o.SessionID == "" || o.ToolName == "" ||
		o.TargetID == "" || o.RequestedBy == "" {
		return errors.New("structured mutation invocation, Run, Session, tool, target, and requester are required")
	}
	if (o.LeaseID == "") != (o.LeaseGeneration == 0) || o.LeaseGeneration < 0 {
		return errors.New("structured mutation execution lease identity and generation are inconsistent")
	}
	if requireSupervisorLease && o.RequestedBy == "run_supervisor" && o.LeaseID == "" {
		return errors.New("supervisor structured mutation requires a Run execution lease")
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) {
		return errors.New("structured mutation key and request fingerprints must be lowercase SHA-256 digests")
	}
	if !o.TargetKind.Valid() {
		return fmt.Errorf("invalid structured mutation target kind %q", o.TargetKind)
	}
	if o.CreatedAt.IsZero() {
		return errors.New("structured mutation creation time is required")
	}
	return nil
}

func OperationKeyDigest(toolName string, runID string, operationKey string) string {
	return Fingerprint("structured_tool_operation.v1", toolName, runID, operationKey)
}

func RunCreationOperationDigest(operationKey string) string {
	return Fingerprint("run_creation_operation.v1", operationKey)
}

func RunCreationRequestFingerprint(goal string, workspaceID string, profile string,
	surface string, phase string, requestedBy string,
) string {
	return Fingerprint("run_creation_request.v1", goal, workspaceID, profile,
		surface, phase, requestedBy)
}

func RunLifecycleOperationDigest(runID string, operationKey string) string {
	return Fingerprint("run_lifecycle_operation.v1", runID, operationKey)
}

func RunLifecycleRequestFingerprint(runID string, action string, expectedStatus string,
	requestedBy string,
) string {
	return Fingerprint("run_lifecycle_request.v1", runID, action, expectedStatus,
		requestedBy)
}

func RunExecutionHandoffOperationDigest(runID string, operationKey string) string {
	return Fingerprint("run_execution_handoff_operation.v1", runID, operationKey)
}

func RunExecutionHandoffRequestFingerprint(runID string, requestedBy string,
	maxSteps int,
) string {
	return Fingerprint("run_execution_handoff_request.v1", runID, requestedBy,
		strconv.Itoa(maxSteps))
}

func RunWakeOperationDigest(runID string, operationKey string) string {
	return Fingerprint("run_wake_operation.v1", runID, operationKey)
}

func RunWakeScheduleRequestFingerprint(runID string, requestedBy string,
	maxAttempts int, initialDelaySeconds int, baseBackoffSeconds int,
	maxBackoffSeconds int, maxElapsedSeconds int,
) string {
	return Fingerprint("run_wake_schedule_request.v1", runID, requestedBy,
		strconv.Itoa(maxAttempts), strconv.Itoa(initialDelaySeconds),
		strconv.Itoa(baseBackoffSeconds), strconv.Itoa(maxBackoffSeconds),
		strconv.Itoa(maxElapsedSeconds))
}

func RunWakeCancelRequestFingerprint(runID string, requestedBy string) string {
	return Fingerprint("run_wake_cancel_request.v1", runID, requestedBy)
}

func RunWakeConsumptionOperationKey(intentID string, generation int) string {
	return "wake-consume-" + Fingerprint("run_wake_consumption_handoff.v1",
		intentID, strconv.Itoa(generation))
}

func FileEditApplyOperationDigest(runID string, editID string,
	operationKey string,
) string {
	return Fingerprint("file_edit_apply_operation.v1", runID, editID, operationKey)
}

func FileEditApplyRequestFingerprint(runID string, editID string,
	appliedBy string,
) string {
	return Fingerprint("file_edit_apply_request.v1", runID, editID, appliedBy)
}

func EvidenceAttachmentOperationDigest(runID string, operationKey string) string {
	return Fingerprint("session_evidence_attachment_operation.v1", runID, operationKey)
}

func EvidenceAttachmentRequestFingerprint(runID string, workspaceID string,
	sourceKind string, sourceRef string, contentSHA256 string, attachedBy string,
) string {
	return Fingerprint("session_evidence_attachment_request.v1", runID, workspaceID,
		sourceKind, sourceRef, contentSHA256, attachedBy)
}

func SupervisorToolOperationKey(runID string, turn int, toolName string, payloadJSON string) string {
	return Fingerprint("supervisor_structured_tool.v1", strings.TrimSpace(runID), strconv.Itoa(turn),
		strings.TrimSpace(toolName), strings.TrimSpace(payloadJSON))
}

func SupervisorToolCallID(operationKey string, round int) (string, error) {
	operationKey = strings.TrimSpace(operationKey)
	if !validDigest(operationKey) {
		return "", errors.New("supervisor tool operation key must be a SHA-256 digest")
	}
	if round <= 0 {
		return "", errors.New("supervisor tool call round must be positive")
	}
	identity := Fingerprint("supervisor_tool_call_id.v1", operationKey, strconv.Itoa(round))
	return "toolu_" + identity[:24], nil
}

func Fingerprint(parts ...string) string {
	hash := sha256.New()
	var size [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(size[:], uint64(len([]byte(part))))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
