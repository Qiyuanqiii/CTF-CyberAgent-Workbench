package httpapi

import (
	"net/http"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/repository"
	"cyberagent-workbench/internal/verification"
)

const (
	VerificationEvidencePathTemplate = "/api/v1/runs/{run_id}/verification-evidence"
	CodeHandoffPathTemplate          = "/api/v1/runs/{run_id}/code-handoff"
	MaxVerificationEvidenceBodyBytes = 16 * 1024
)

func (a *API) workspaceRepositoryDiff(request *http.Request,
	workspaceID string,
) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	registered, err := a.store.GetWorkspaceInfo(request.Context(), workspaceID)
	if err != nil {
		return nil, nil, apperror.Normalize(err)
	}
	if registered.ID != workspaceID {
		return nil, nil, apperror.New(apperror.CodeInternal,
			"workspace lookup returned a mismatched identity")
	}
	projection, err := repository.InspectDiff(request.Context(), registered.RootPath,
		registered.ID)
	if err != nil {
		return nil, nil, err
	}
	items := make([]RepositoryDiffItemView, len(projection.Items))
	for index, item := range projection.Items {
		items[index] = RepositoryDiffItemView{
			Path: item.Path, Staging: item.Staging, Worktree: item.Worktree,
			ContentState: item.ContentState, Patch: item.Patch, PatchBytes: item.PatchBytes,
			AddedLines: item.AddedLines, DeletedLines: item.DeletedLines,
			Redacted: item.Redacted, Truncated: item.Truncated,
		}
	}
	return RepositoryDiffView{
		ProtocolVersion: projection.ProtocolVersion, WorkspaceID: projection.WorkspaceID,
		Kind: projection.Kind, Available: projection.Available, BaseHead: projection.BaseHead,
		Items: items, ReturnedCount: projection.ReturnedCount,
		OmittedCount: projection.OmittedCount, RedactionCount: projection.RedactionCount,
		TotalPatchBytes: projection.TotalPatchBytes, Truncated: projection.Truncated,
		ReadOnly: projection.ReadOnly, InstructionAuthorized: projection.InstructionAuthorized,
		MutationSupported:    projection.MutationSupported,
		AuthorityGranted:     projection.AuthorityGranted,
		RootPathExposed:      projection.RootPathExposed,
		RawContentIncluded:   projection.RawContentIncluded,
		PatchContentIncluded: projection.PatchContentIncluded,
		RemoteConfigIncluded: projection.RemoteConfigIncluded,
		ProcessStarted:       projection.ProcessStarted, NetworkUsed: projection.NetworkUsed,
		HooksExecuted: projection.HooksExecuted,
	}, nil, nil
}

func (a *API) runVerificationEvidence(request *http.Request,
	runID string,
) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	inventory, err := application.NewVerificationEvidenceService(a.store).Inventory(
		request.Context(), runID)
	if err != nil {
		return nil, nil, err
	}
	items := make([]VerificationEvidenceItemView, len(inventory.Items))
	for index, value := range inventory.Items {
		items[index] = verificationEvidenceItemView(value)
	}
	return VerificationEvidenceInventoryView{
		ProtocolVersion: inventory.ProtocolVersion, RunID: inventory.RunID,
		SessionID: inventory.SessionID, WorkspaceID: inventory.WorkspaceID,
		Items: items, PassCount: inventory.PassCount, FailCount: inventory.FailCount,
		UnknownCount: inventory.UnknownCount, Truncated: inventory.Truncated,
	}, nil, nil
}

func (a *API) runCodeHandoff(request *http.Request, runID string) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	value, err := application.NewCodeHandoffService(a.store).Build(request.Context(), runID)
	if err != nil {
		return nil, nil, err
	}
	return codeHandoffView(value), nil, nil
}

func matchVerificationEvidencePath(requestPath string) (string, bool) {
	const prefix = "/api/v1/runs/"
	const suffix = "/verification-evidence"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	return runID, runID != "" && !strings.Contains(runID, "/")
}

func (a *API) serveVerificationEvidenceControl(writer http.ResponseWriter,
	request *http.Request, requestID string, runID string,
) {
	const label = "Verification evidence"
	if !a.authorizeRunOperation(writer, request, requestID,
		a.verificationEvidenceEnabled, label) {
		return
	}
	if err := validatePathIdentity(runID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := validateJSONContentType(request.Header); err != nil {
		a.writeError(writer, requestID, err, http.StatusUnsupportedMediaType)
		return
	}
	operationKey, err := sessionControlIdempotencyKey(request.Header, label)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if err := rejectQuery(request.URL.Query()); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	body, err := readBoundedRequestBody(request, MaxVerificationEvidenceBodyBytes)
	if err != nil {
		a.writeError(writer, requestID, err, runOperationErrorStatus(err))
		return
	}
	if err := rejectDuplicateJSONObjectFields(body, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	var view VerificationEvidenceRequestView
	if err := decodeStrictRunOperation(body, &view, label); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	result, err := application.NewVerificationEvidenceService(a.store).Record(
		request.Context(), application.RecordVerificationEvidenceRequest{
			Version: view.Version, RunID: runID, Outcome: view.Outcome,
			Title: view.Title, Summary: view.Summary, OperationKey: operationKey,
			RecordedBy: "http_run_operator",
		})
	if err != nil {
		a.writeError(writer, requestID, apperror.Normalize(err), 0)
		return
	}
	a.writeSuccessStatus(writer, requestID,
		verificationEvidenceControlView(result.Evidence, result.Replayed), nil,
		http.StatusAccepted)
}

func verificationEvidenceItemView(value verification.Evidence) VerificationEvidenceItemView {
	return VerificationEvidenceItemView{
		ProtocolVersion: value.ProtocolVersion, ID: value.ID, RunID: value.RunID,
		SessionID: value.SessionID, WorkspaceID: value.WorkspaceID,
		Outcome: string(value.Outcome), Title: value.Title, Summary: value.Summary,
		SummarySHA256: value.SummarySHA256, Redacted: value.Redacted,
		RecordedAt: value.CreatedAt, Immutable: true, OperatorSupplied: true,
	}
}

func verificationEvidenceControlView(value verification.Evidence,
	replayed bool,
) VerificationEvidenceControlView {
	return VerificationEvidenceControlView{
		ProtocolVersion: value.ProtocolVersion, ID: value.ID, RunID: value.RunID,
		SessionID: value.SessionID, WorkspaceID: value.WorkspaceID,
		Outcome: string(value.Outcome), Title: value.Title, Summary: value.Summary,
		SummarySHA256: value.SummarySHA256, Redacted: value.Redacted,
		RecordedAt: value.CreatedAt, Immutable: true, OperatorSupplied: true,
		Replayed: replayed,
	}
}

func codeHandoffView(value application.CodeHandoff) CodeHandoffView {
	verificationReferences := make([]CodeHandoffVerificationReferenceView,
		len(value.Verification.References))
	for index, reference := range value.Verification.References {
		verificationReferences[index] = CodeHandoffVerificationReferenceView{
			ID: reference.ID, Outcome: string(reference.Outcome), Redacted: reference.Redacted,
			RecordedAt: reference.CreatedAt,
		}
	}
	actions := make([]CodeHandoffActionReferenceView, len(value.PendingActions))
	for index, action := range value.PendingActions {
		actions[index] = CodeHandoffActionReferenceView{
			ID: action.ID, Kind: action.Kind, State: action.State,
			Destination: action.Destination, AvailableAt: action.AvailableAt, DueAt: action.DueAt,
		}
	}
	reports := make([]CodeHandoffReportReferenceView, len(value.ReportReferences))
	for index, report := range value.ReportReferences {
		reports[index] = CodeHandoffReportReferenceView{
			ID: report.ID, Status: string(report.Status), FindingCount: report.FindingCount,
			CreatedAt: report.CreatedAt,
		}
	}
	return CodeHandoffView{
		ProtocolVersion: value.ProtocolVersion, RunID: value.RunID,
		MissionID: value.MissionID, SessionID: value.SessionID,
		WorkspaceID: value.WorkspaceID, RunStatus: string(value.RunStatus),
		Surface: string(value.Surface), Phase: string(value.Phase),
		ModeRevision: value.ModeRevision, GeneratedAt: value.GeneratedAt,
		Plan: CodeHandoffPlanView{
			State: value.Plan.State, ProposalID: value.Plan.ProposalID,
			SelectionID: value.Plan.SelectionID, DirectionCount: value.Plan.DirectionCount,
			SelectedDirection: value.Plan.SelectedDirection, ModuleCount: value.Plan.ModuleCount,
			PendingCount: value.Plan.PendingCount, InProgressCount: value.Plan.InProgressCount,
			BlockedCount: value.Plan.BlockedCount, CompletedCount: value.Plan.CompletedCount,
			CancelledCount: value.Plan.CancelledCount,
		},
		Queue: CodeHandoffQueueView{Pending: value.Queue.Pending, Prepared: value.Queue.Prepared,
			Committed: value.Queue.Committed, Cancelled: value.Queue.Cancelled},
		ChangeSet: CodeHandoffChangeSetView{
			Proposed: value.ChangeSet.Proposed, Approved: value.ChangeSet.Approved,
			Applied: value.ChangeSet.Applied, Denied: value.ChangeSet.Denied,
			Failed: value.ChangeSet.Failed, ReturnedCount: value.ChangeSet.ReturnedCount,
			TotalDiffBytes: value.ChangeSet.TotalDiffBytes, Truncated: value.ChangeSet.Truncated,
		},
		Verification: CodeHandoffVerificationView{
			PassCount: value.Verification.PassCount, FailCount: value.Verification.FailCount,
			UnknownCount:  value.Verification.UnknownCount,
			ReturnedCount: value.Verification.ReturnedCount,
			Truncated:     value.Verification.Truncated, References: verificationReferences,
		},
		PendingActionCount:      value.PendingActionCount,
		PendingActionsTruncated: value.PendingActionsTruncated, PendingActions: actions,
		ReportReferencesTruncated: value.ReportReferencesTruncated,
		ReportReferences:          reports, Regenerable: value.Regenerable,
		DurableSources:        value.DurableSources,
		PrivateBodiesIncluded: value.PrivateBodiesIncluded,
		CompositeMutation:     value.CompositeMutation, ResumeAuthorized: value.ResumeAuthorized,
		ExecutionStarted: value.ExecutionStarted,
	}
}
