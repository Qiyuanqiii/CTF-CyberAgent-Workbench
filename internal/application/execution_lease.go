package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

func withRunExecutionLease(ctx context.Context, store RunExecutionLeaseStore,
	runID string, ownerID string, leasePolicy RunExecutionLeasePolicy,
	operation func(context.Context, domain.RunExecutionLease) error,
) error {
	if store == nil || operation == nil {
		return apperror.New(apperror.CodeFailedPrecondition,
			"run execution lease dependencies are required")
	}
	if err := leasePolicy.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"invalid run execution lease policy", err)
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return apperror.New(apperror.CodeFailedPrecondition,
			"run execution lease owner is required")
	}
	acquired, err := store.AcquireRunExecutionLease(ctx,
		domain.AcquireRunExecutionLeaseRequest{
			RunID: strings.TrimSpace(runID), OwnerID: ownerID, TTL: leasePolicy.TTL,
		})
	if err != nil {
		return apperror.Normalize(err)
	}
	leaseCtx, cancelLease := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan struct{})
	heartbeatErr := make(chan error, 1)
	go func() {
		defer close(done)
		renewRunExecutionLease(leaseCtx, store, acquired.Lease, leasePolicy,
			stop, heartbeatErr, cancelLease)
	}()
	operationErr := operation(leaseCtx, acquired.Lease)
	close(stop)
	cancelLease()
	<-done
	var renewalErr error
	select {
	case renewalErr = <-heartbeatErr:
	default:
	}
	releaseCtx, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	_, _, releaseErr := store.ReleaseRunExecutionLease(releaseCtx, acquired.Lease)
	cancelRelease()
	if renewalErr != nil {
		return errors.Join(apperror.Normalize(renewalErr), operationErr, releaseErr)
	}
	return errors.Join(operationErr, releaseErr)
}

func renewRunExecutionLease(ctx context.Context, store RunExecutionLeaseStore,
	lease domain.RunExecutionLease, policy RunExecutionLeasePolicy, stop <-chan struct{},
	heartbeatErr chan<- error, cancel context.CancelFunc,
) {
	ticker := time.NewTicker(policy.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			timeout := minDuration(2*time.Second, policy.RenewInterval)
			renewCtx, cancelRenew := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			_, err := store.RenewRunExecutionLease(renewCtx, lease, policy.TTL)
			cancelRenew()
			if err != nil {
				select {
				case heartbeatErr <- err:
				default:
				}
				cancel()
				return
			}
		}
	}
}

func minDuration(left time.Duration, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
