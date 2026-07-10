package application

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/redact"
)

const maxRootActionJSONBytes = 64 * 1024

func parseRootAction(raw string) (domain.RootAction, error) {
	if len(raw) > maxRootActionJSONBytes {
		return domain.RootAction{}, apperror.New(apperror.CodeResourceExhausted, "provider root lifecycle action exceeds 65536 bytes")
	}
	if !utf8.ValidString(raw) {
		return domain.RootAction{}, apperror.New(apperror.CodeFailedPrecondition, "provider root lifecycle action is not valid UTF-8")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return domain.RootAction{}, apperror.New(apperror.CodeFailedPrecondition, "provider returned an empty root lifecycle action")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var action domain.RootAction
	if err := decoder.Decode(&action); err != nil {
		return domain.RootAction{}, apperror.Wrap(apperror.CodeFailedPrecondition, "provider returned invalid root lifecycle JSON", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values are not allowed")
		}
		return domain.RootAction{}, apperror.Wrap(apperror.CodeFailedPrecondition, "provider returned trailing root lifecycle data", err)
	}
	action.Version = strings.TrimSpace(action.Version)
	action.Message = strings.TrimSpace(action.Message)
	action.Summary = strings.TrimSpace(action.Summary)
	action.Reason = strings.TrimSpace(action.Reason)
	if err := action.Validate(); err != nil {
		return domain.RootAction{}, apperror.Wrap(apperror.CodeFailedPrecondition, "provider returned an invalid root lifecycle action", err)
	}
	return action, nil
}

func redactRootAction(action domain.RootAction) domain.RootAction {
	action.Message = redact.String(action.Message)
	action.Summary = redact.String(action.Summary)
	action.Reason = redact.String(action.Reason)
	return action
}

func rootActionPolicyText(action domain.RootAction) string {
	return strings.TrimSpace(strings.Join([]string{action.Message, action.Summary, action.Reason}, "\n"))
}
