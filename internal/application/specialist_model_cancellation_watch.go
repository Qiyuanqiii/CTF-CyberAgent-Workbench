package application

import (
	"context"
	"sync"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
)

func (r *SpecialistRunner) watchSpecialistModelCancellation(parent context.Context,
	ref domain.AgentAttemptRef, attempt llm.ModelAttempt, cancelCall context.CancelFunc,
) func() {
	if r == nil || r.store == nil || cancelCall == nil ||
		r.cancellationPollInterval <= 0 {
		return func() {}
	}
	watchCtx, cancelWatch := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(r.cancellationPollInterval)
		defer ticker.Stop()
		for {
			observed, stop := r.pollSpecialistModelCancellation(watchCtx, ref, attempt)
			if observed {
				cancelCall()
				return
			}
			if stop {
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
			cancelWatch()
			<-done
		})
	}
}

func (r *SpecialistRunner) pollSpecialistModelCancellation(ctx context.Context,
	ref domain.AgentAttemptRef, attempt llm.ModelAttempt,
) (bool, bool) {
	_, observed, err := r.store.ObserveSpecialistModelCancellation(ctx, ref, attempt)
	if err == nil {
		return observed, false
	}
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
