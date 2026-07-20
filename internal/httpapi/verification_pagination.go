package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/verification"
)

const verificationCoverageCursorVersion = 2

type verificationCoveragePageRequest struct {
	Limit  int
	Scope  string
	Anchor application.VerificationCoveragePageAnchor
}

type verificationCoverageCursor struct {
	Version             int    `json:"v"`
	Scope               string `json:"s"`
	HighWaterSequence   int64  `json:"h"`
	BeforeEventSequence int64  `json:"b"`
	BeforeAssociationID string `json:"i"`
	Consumed            int    `json:"c"`
}

func parseVerificationCoveragePage(values url.Values,
	resourcePath string,
) (verificationCoveragePageRequest, error) {
	limit := DefaultPageLimit
	if raw, ok := singleQueryValue(values, "limit"); ok {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > MaxPageLimit {
			return verificationCoveragePageRequest{}, apperror.New(
				apperror.CodeInvalidArgument,
				fmt.Sprintf("page limit must be between 1 and %d", MaxPageLimit))
		}
		limit = parsed
	}
	result := verificationCoveragePageRequest{
		Limit: limit,
		Scope: pageScope(resourcePath, values),
	}
	if raw, ok := singleQueryValue(values, "cursor"); ok {
		cursor, err := decodeVerificationCoverageCursor(raw, result.Scope)
		if err != nil {
			return verificationCoveragePageRequest{}, err
		}
		result.Anchor = application.VerificationCoveragePageAnchor{
			SnapshotHighWaterEventSequence: cursor.HighWaterSequence,
			BeforeEventSequence:            cursor.BeforeEventSequence,
			BeforeAssociationID:            cursor.BeforeAssociationID,
			Consumed:                       cursor.Consumed,
		}
	}
	return result, nil
}

func decodeVerificationCoverageCursor(encoded string,
	expectedScope string,
) (verificationCoverageCursor, error) {
	invalid := func(message string) (verificationCoverageCursor, error) {
		return verificationCoverageCursor{}, apperror.New(apperror.CodeInvalidArgument, message)
	}
	if len(encoded) == 0 || len(encoded) > MaxCursorBytes {
		return invalid("verification coverage cursor is invalid")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 || len(raw) > MaxCursorBytes {
		return invalid("verification coverage cursor is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor verificationCoverageCursor
	if err := decoder.Decode(&cursor); err != nil {
		return invalid("verification coverage cursor is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return invalid("verification coverage cursor is invalid")
	}
	if cursor.Version != verificationCoverageCursorVersion || cursor.Scope != expectedScope ||
		cursor.HighWaterSequence <= 0 || cursor.BeforeEventSequence <= 0 ||
		cursor.BeforeEventSequence > cursor.HighWaterSequence ||
		!domain.ValidAgentID(cursor.BeforeAssociationID) || cursor.Consumed <= 0 ||
		cursor.Consumed >= verification.MaxCoveragePageWindow {
		return invalid("verification coverage cursor does not match this exact item snapshot")
	}
	return cursor, nil
}

func encodeVerificationCoverageCursor(scope string,
	detail application.VerificationPlanItemCoverageDetail,
) (string, bool) {
	if detail.SnapshotHighWaterEventSequence <= 0 ||
		detail.NextPageBeforeEventSequence <= 0 ||
		detail.NextPageBeforeEventSequence > detail.SnapshotHighWaterEventSequence ||
		!domain.ValidAgentID(detail.NextPageBeforeAssociationID) ||
		detail.NextPageConsumed <= 0 ||
		detail.NextPageConsumed >= verification.MaxCoveragePageWindow {
		return "", false
	}
	raw, err := json.Marshal(verificationCoverageCursor{
		Version: verificationCoverageCursorVersion, Scope: scope,
		HighWaterSequence:   detail.SnapshotHighWaterEventSequence,
		BeforeEventSequence: detail.NextPageBeforeEventSequence,
		BeforeAssociationID: detail.NextPageBeforeAssociationID,
		Consumed:            detail.NextPageConsumed,
	})
	if err != nil || len(raw) == 0 || len(raw) > MaxCursorBytes {
		return "", false
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded, len(encoded) <= MaxCursorBytes
}

func verificationCoveragePage(detail application.VerificationPlanItemCoverageDetail,
	request verificationCoveragePageRequest,
) *Page {
	page := &Page{Limit: request.Limit}
	if !detail.AssociationsTruncated {
		return page
	}
	if cursor, ok := encodeVerificationCoverageCursor(request.Scope, detail); ok {
		page.NextCursor = cursor
	} else {
		page.Truncated = true
	}
	return page
}
