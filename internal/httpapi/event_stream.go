package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/events"
)

const (
	RunEventStreamVersion      = "run-events.v1"
	RunEventStreamPathTemplate = "/api/v1/runs/{run_id}/events/stream"
	MaxEventStreamCursorBytes  = 512
	MaxEventStreamFrameBytes   = 2 * 1024 * 1024
	MaxEventStreamBatchSize    = 32

	DefaultEventStreamPollInterval      = 250 * time.Millisecond
	DefaultEventStreamHeartbeatInterval = 10 * time.Second
	DefaultEventStreamMaxDuration       = 5 * time.Minute
	DefaultEventStreamWriteTimeout      = 2 * time.Second
	DefaultEventStreamBatchSize         = 32
	DefaultEventStreamMaxEvents         = 10000
	DefaultEventStreamMaxConnections    = 16
)

type EventStreamConfig struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	MaxDuration       time.Duration
	WriteTimeout      time.Duration
	BatchSize         int
	MaxEvents         int
	MaxConnections    int
}

type RunEventStreamView struct {
	Version   string    `json:"version"`
	RequestID string    `json:"request_id"`
	RunID     string    `json:"run_id"`
	Cursor    string    `json:"cursor"`
	Sequence  int64     `json:"sequence"`
	Event     EventView `json:"event"`
}

type eventStreamCursor struct {
	Version  int    `json:"v"`
	Sequence int64  `json:"s"`
	Scope    string `json:"r"`
}

func normalizeEventStreamConfig(config EventStreamConfig) (EventStreamConfig, error) {
	if config.PollInterval == 0 {
		config.PollInterval = DefaultEventStreamPollInterval
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = DefaultEventStreamHeartbeatInterval
	}
	if config.MaxDuration == 0 {
		config.MaxDuration = DefaultEventStreamMaxDuration
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = DefaultEventStreamWriteTimeout
	}
	if config.BatchSize == 0 {
		config.BatchSize = DefaultEventStreamBatchSize
	}
	if config.MaxEvents == 0 {
		config.MaxEvents = DefaultEventStreamMaxEvents
	}
	if config.MaxConnections == 0 {
		config.MaxConnections = DefaultEventStreamMaxConnections
	}
	if config.PollInterval < 5*time.Millisecond || config.PollInterval > 5*time.Second {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			"event stream poll interval must be between 5ms and 5s")
	}
	if config.HeartbeatInterval < 10*time.Millisecond || config.HeartbeatInterval > time.Minute {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			"event stream heartbeat interval must be between 10ms and 1m")
	}
	if config.MaxDuration < 25*time.Millisecond || config.MaxDuration > 30*time.Minute {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			"event stream maximum duration must be between 25ms and 30m")
	}
	if config.WriteTimeout < 5*time.Millisecond || config.WriteTimeout > 4*time.Second {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			"event stream write timeout must be between 5ms and 4s")
	}
	if config.BatchSize <= 0 || config.BatchSize > MaxEventStreamBatchSize {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			fmt.Sprintf("event stream batch size must be between 1 and %d", MaxEventStreamBatchSize))
	}
	if config.MaxEvents <= 0 || config.MaxEvents > 100000 {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			"event stream event limit must be between 1 and 100000")
	}
	if config.MaxConnections <= 0 || config.MaxConnections > 128 {
		return EventStreamConfig{}, apperror.New(apperror.CodeInvalidArgument,
			"event stream connection limit must be between 1 and 128")
	}
	return config, nil
}

func matchRunEventStreamPath(requestPath string) (string, bool) {
	const prefix = "/api/v1/runs/"
	const suffix = "/events/stream"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	if runID == "" || strings.Contains(runID, "/") {
		return "", false
	}
	return runID, true
}

func (a *API) serveRunEventStream(writer http.ResponseWriter, request *http.Request,
	requestID string, runID string,
) {
	afterSequence, err := parseEventStreamCursor(request, runID)
	if err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	select {
	case a.eventStreamSlots <- struct{}{}:
		defer func() { <-a.eventStreamSlots }()
	default:
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeResourceExhausted, "event stream connection limit reached"), 0)
		return
	}
	if _, err := a.store.GetRun(request.Context(), runID); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	limit := min(a.eventStream.BatchSize, a.eventStream.MaxEvents)
	batch, err := a.store.ListRunEventsAfterSequence(request.Context(), runID, afterSequence, limit)
	if err != nil {
		if request.Context().Err() == nil {
			a.writeError(writer, requestID, err, 0)
		}
		return
	}
	if err := validateEventStreamBatch(batch, runID, afterSequence); err != nil {
		a.writeError(writer, requestID, err, 0)
		return
	}
	if request.Context().Err() != nil {
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	streamDeadline := time.Now().Add(a.eventStream.MaxDuration)
	duration := time.NewTimer(time.Until(streamDeadline))
	defer duration.Stop()
	if err := writeEventStreamFrame(writer, boundedStreamWriteTimeout(a.eventStream.WriteTimeout, streamDeadline),
		[]byte(": cyberagent "+RunEventStreamVersion+"\nretry: 1000\n\n")); err != nil {
		return
	}

	emitted := 0
	afterSequence, emitted, err = a.emitRunEventBatch(writer, requestID, runID,
		afterSequence, emitted, batch, streamDeadline)
	if err != nil || emitted >= a.eventStream.MaxEvents {
		return
	}

	poll := time.NewTicker(a.eventStream.PollInterval)
	heartbeat := time.NewTicker(a.eventStream.HeartbeatInterval)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-duration.C:
			return
		case <-heartbeat.C:
			if err := writeEventStreamFrame(writer,
				boundedStreamWriteTimeout(a.eventStream.WriteTimeout, streamDeadline), []byte(": heartbeat\n\n")); err != nil {
				return
			}
		case <-poll.C:
			remaining := a.eventStream.MaxEvents - emitted
			limit := min(a.eventStream.BatchSize, remaining)
			batch, err := a.store.ListRunEventsAfterSequence(request.Context(), runID, afterSequence, limit)
			if err != nil || validateEventStreamBatch(batch, runID, afterSequence) != nil {
				return
			}
			afterSequence, emitted, err = a.emitRunEventBatch(writer, requestID, runID,
				afterSequence, emitted, batch, streamDeadline)
			if err != nil || emitted >= a.eventStream.MaxEvents {
				return
			}
		}
	}
}

func (a *API) emitRunEventBatch(writer http.ResponseWriter, requestID string, runID string,
	afterSequence int64, emitted int, batch []events.Event, streamDeadline time.Time,
) (int64, int, error) {
	for _, event := range batch {
		if !time.Now().Before(streamDeadline) {
			return afterSequence, emitted, context.DeadlineExceeded
		}
		cursor := encodeEventStreamCursor(runID, event.Sequence)
		data, err := json.Marshal(RunEventStreamView{Version: RunEventStreamVersion,
			RequestID: requestID, RunID: runID, Cursor: cursor, Sequence: event.Sequence,
			Event: eventView(event)})
		if err != nil {
			return afterSequence, emitted, err
		}
		frame := make([]byte, 0, len(cursor)+len(data)+32)
		frame = append(frame, "id: "...)
		frame = append(frame, cursor...)
		frame = append(frame, "\nevent: run.event\ndata: "...)
		frame = append(frame, data...)
		frame = append(frame, '\n', '\n')
		if err := writeEventStreamFrame(writer,
			boundedStreamWriteTimeout(a.eventStream.WriteTimeout, streamDeadline), frame); err != nil {
			return afterSequence, emitted, err
		}
		afterSequence = event.Sequence
		emitted++
	}
	return afterSequence, emitted, nil
}

func validateEventStreamBatch(batch []events.Event, runID string, afterSequence int64) error {
	expected := afterSequence + 1
	for _, event := range batch {
		if event.RunID != runID || event.Sequence != expected {
			return apperror.New(apperror.CodeInternal, "event stream sequence is inconsistent")
		}
		if err := event.Validate(); err != nil {
			return apperror.New(apperror.CodeInternal, "event stream contains an invalid event")
		}
		expected++
	}
	return nil
}

func parseEventStreamCursor(request *http.Request, runID string) (int64, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "cursor"); err != nil {
		return 0, err
	}
	queryCursor, queryPresent := singleQueryValue(values, "cursor")
	if queryPresent && queryCursor == "" {
		return 0, apperror.New(apperror.CodeInvalidArgument, "event stream cursor is invalid")
	}
	headerValues := request.Header.Values("Last-Event-ID")
	if len(headerValues) > 1 {
		return 0, apperror.New(apperror.CodeInvalidArgument,
			"Last-Event-ID must appear at most once")
	}
	headerCursor := ""
	if len(headerValues) == 1 {
		headerCursor = strings.TrimSpace(headerValues[0])
		if headerCursor == "" {
			return 0, apperror.New(apperror.CodeInvalidArgument, "Last-Event-ID is invalid")
		}
	}
	if queryPresent && headerCursor != "" {
		return 0, apperror.New(apperror.CodeInvalidArgument,
			"event stream cursor and Last-Event-ID cannot be combined")
	}
	cursor := headerCursor
	if queryPresent {
		cursor = queryCursor
	}
	if cursor == "" {
		return 0, nil
	}
	return decodeEventStreamCursor(cursor, runID)
}

func encodeEventStreamCursor(runID string, sequence int64) string {
	raw, _ := json.Marshal(eventStreamCursor{Version: 1, Sequence: sequence, Scope: eventStreamScope(runID)})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeEventStreamCursor(encoded string, runID string) (int64, error) {
	if encoded == "" || len(encoded) > MaxEventStreamCursorBytes {
		return 0, apperror.New(apperror.CodeInvalidArgument, "event stream cursor is invalid")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) == 0 || len(raw) > MaxEventStreamCursorBytes {
		return 0, apperror.New(apperror.CodeInvalidArgument, "event stream cursor is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor eventStreamCursor
	if err := decoder.Decode(&cursor); err != nil {
		return 0, apperror.New(apperror.CodeInvalidArgument, "event stream cursor is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, apperror.New(apperror.CodeInvalidArgument, "event stream cursor is invalid")
	}
	if cursor.Version != 1 || cursor.Sequence < 0 || cursor.Scope != eventStreamScope(runID) {
		return 0, apperror.New(apperror.CodeInvalidArgument,
			"event stream cursor does not match this Run")
	}
	return cursor.Sequence, nil
}

func eventStreamScope(runID string) string {
	digest := sha256.Sum256([]byte(runID))
	return hex.EncodeToString(digest[:16])
}

func writeEventStreamFrame(writer http.ResponseWriter, timeout time.Duration, frame []byte) error {
	if len(frame) == 0 || len(frame) > MaxEventStreamFrameBytes {
		return apperror.New(apperror.CodeResourceExhausted, "event stream frame exceeds its limit")
	}
	controller := http.NewResponseController(writer)
	deadlineSet := false
	if err := controller.SetWriteDeadline(time.Now().Add(timeout)); err == nil {
		deadlineSet = true
	} else if !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if deadlineSet {
		defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	}
	written, err := writer.Write(frame)
	if err != nil {
		return err
	}
	if written != len(frame) {
		return io.ErrShortWrite
	}
	if err := controller.Flush(); err != nil {
		return err
	}
	return nil
}

func boundedStreamWriteTimeout(configured time.Duration, deadline time.Time) time.Duration {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Nanosecond
	}
	if remaining < configured {
		return remaining
	}
	return configured
}
