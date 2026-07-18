package httpapi

import (
	"net/http"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
)

func (a *API) runOperatorActions(request *http.Request,
	runID string,
) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	store, ok := a.store.(application.OperatorActionCenterStore)
	if !ok {
		return nil, nil, apperror.New(apperror.CodeFailedPrecondition,
			"operator action center is unavailable")
	}
	center, err := application.NewOperatorActionCenterService(store).List(
		request.Context(), runID)
	if err != nil {
		return nil, nil, err
	}
	items := make([]OperatorActionItemView, len(center.Items))
	for index, item := range center.Items {
		items[index] = OperatorActionItemView{ID: item.ID, Kind: item.Kind,
			State: item.State, Destination: item.Destination,
			AvailableAt: item.AvailableAt, DueAt: item.DueAt}
	}
	return OperatorActionCenterView{ProtocolVersion: center.ProtocolVersion,
		RunID: center.RunID, GeneratedAt: center.GeneratedAt,
		Items: items, Truncated: center.Truncated}, nil, nil
}

func (a *API) runEvidenceInventory(request *http.Request,
	runID string,
) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	store, ok := a.store.(application.EvidenceInventoryStore)
	if !ok {
		return nil, nil, apperror.New(apperror.CodeFailedPrecondition,
			"evidence inventory is unavailable")
	}
	inventory, err := application.NewEvidenceInventoryService(store).List(
		request.Context(), runID)
	if err != nil {
		return nil, nil, err
	}
	items := make([]EvidenceInventoryItemView, len(inventory.Items))
	for index, item := range inventory.Items {
		items[index] = EvidenceInventoryItemView{
			AttachmentID: item.AttachmentID, RunID: item.RunID,
			SessionID: item.SessionID, WorkspaceID: item.WorkspaceID,
			SourceKind: item.SourceKind, SourceRef: item.SourceRef,
			ContentSHA256:         item.ContentSHA256,
			InstructionAuthorized: item.InstructionAuthorized,
			AttachedAt:            item.AttachedAt,
		}
	}
	return EvidenceInventoryView{ProtocolVersion: inventory.ProtocolVersion,
		RunID: inventory.RunID, Items: items, Truncated: inventory.Truncated}, nil, nil
}
