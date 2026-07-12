package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"cyberagent-workbench/internal/apperror"
)

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 100
	MaxCursorBytes   = 512
	cursorVersion    = 1
)

type Page struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type pageRequest struct {
	Limit  int
	Offset int
	Scope  string
}

type cursorEnvelope struct {
	Version int    `json:"v"`
	Offset  int    `json:"o"`
	Scope   string `json:"s"`
}

func parsePage(values url.Values, resourcePath string) (pageRequest, error) {
	limit := DefaultPageLimit
	if raw, ok := singleQueryValue(values, "limit"); ok {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > MaxPageLimit {
			return pageRequest{}, apperror.New(apperror.CodeInvalidArgument,
				fmt.Sprintf("page limit must be between 1 and %d", MaxPageLimit))
		}
		limit = parsed
	}
	scope := pageScope(resourcePath, values)
	offset := 0
	if raw, ok := singleQueryValue(values, "cursor"); ok {
		decoded, err := decodeCursor(raw, scope)
		if err != nil {
			return pageRequest{}, err
		}
		offset = decoded
	}
	return pageRequest{Limit: limit, Offset: offset, Scope: scope}, nil
}

func singleQueryValue(values url.Values, key string) (string, bool) {
	items, ok := values[key]
	if !ok {
		return "", false
	}
	if len(items) != 1 || strings.TrimSpace(items[0]) == "" {
		return "", true
	}
	return strings.TrimSpace(items[0]), true
}

func validateSingleQueryValues(values url.Values, keys ...string) error {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key, items := range values {
		if _, ok := allowed[key]; !ok {
			return apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("unknown query parameter %q", key))
		}
		if (key == "limit" || key == "cursor" || key == "mission_id" || key == "owner" ||
			key == "owner_agent_id" ||
			key == "pinned" || key == "source_id" || key == "stream" || key == "include_compacted") && len(items) != 1 {
			return apperror.New(apperror.CodeInvalidArgument,
				fmt.Sprintf("query parameter %q must appear exactly once", key))
		}
	}
	return nil
}

func pageScope(resourcePath string, values url.Values) string {
	copyValues := make(url.Values, len(values))
	for key, items := range values {
		if key == "cursor" || key == "limit" {
			continue
		}
		copyValues[key] = append([]string(nil), items...)
	}
	digest := sha256.Sum256([]byte(resourcePath + "\n" + copyValues.Encode()))
	return hex.EncodeToString(digest[:16])
}

func decodeCursor(encoded string, expectedScope string) (int, error) {
	if len(encoded) == 0 || len(encoded) > MaxCursorBytes {
		return 0, apperror.New(apperror.CodeInvalidArgument, "page cursor is invalid")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 || len(raw) > MaxCursorBytes {
		return 0, apperror.New(apperror.CodeInvalidArgument, "page cursor is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor cursorEnvelope
	if err := decoder.Decode(&cursor); err != nil {
		return 0, apperror.New(apperror.CodeInvalidArgument, "page cursor is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, apperror.New(apperror.CodeInvalidArgument, "page cursor is invalid")
	}
	if cursor.Version != cursorVersion || cursor.Offset < 0 || cursor.Offset > maxStoreCursorOffset ||
		cursor.Scope != expectedScope {
		return 0, apperror.New(apperror.CodeInvalidArgument,
			"page cursor does not match this resource and filter")
	}
	return cursor.Offset, nil
}

const maxStoreCursorOffset = 100000

func encodeCursor(scope string, offset int) string {
	raw, _ := json.Marshal(cursorEnvelope{Version: cursorVersion, Offset: offset, Scope: scope})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func trimPage[T any](items []T, request pageRequest) ([]T, *Page) {
	hasMore := len(items) > request.Limit
	if hasMore {
		items = items[:request.Limit]
	}
	page := &Page{Limit: request.Limit}
	if hasMore {
		nextOffset := request.Offset + request.Limit
		if nextOffset <= maxStoreCursorOffset {
			page.NextCursor = encodeCursor(request.Scope, nextOffset)
		} else {
			page.Truncated = true
		}
	}
	return items, page
}
