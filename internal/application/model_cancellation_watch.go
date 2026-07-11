package application

import (
	"context"
	"sync"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
)

func (s *RunSupervisor) watchModelCancellation(parent context.Context,
	checkpoint domain.SupervisorCheckpoint, attempt llm.ModelAttempt, live *activeCallLease,
) func() {
	if s == nil || s.store == nil || live == nil || s.cancellationPollInterval <= 0 {
		return func() {}
	}
	watchCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.cancellationPollInterval)
		defer ticker.Stop()
		for {
			observed, stop := s.pollModelCancellation(watchCtx, checkpoint, attempt, live)
			if observed || stop {
				return
			}
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (s *RunSupervisor) pollModelCancellation(ctx context.Context, checkpoint domain.SupervisorCheckpoint,
	attempt llm.ModelAttempt, live *activeCallLease,
) (bool, bool) {
	_, observed, err := s.store.ObserveSupervisorModelCancellation(ctx, checkpoint, attempt)
	if err != nil {
		if ctx.Err() != nil {
			return false, true
		}
		switch apperror.CodeOf(apperror.Normalize(err)) {
		case apperror.CodeConflict, apperror.CodeFailedPrecondition, apperror.CodeNotFound,
			apperror.CodeCancelled, apperror.CodeDeadlineExceeded:
			return false, true
		default:
			return false, false
		}
	}
	if !observed {
		return false, false
	}
	return live.signalPersistedCancellation(), true
}
