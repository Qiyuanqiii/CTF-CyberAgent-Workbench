package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MinRunExecutionLeaseTTL  = 100 * time.Millisecond
	MaxRunExecutionLeaseTTL  = 24 * time.Hour
	MaxRunLeaseIdentityRunes = 256
)

type RunExecutionLeaseStatus string

const (
	RunExecutionLeaseActive   RunExecutionLeaseStatus = "active"
	RunExecutionLeaseReleased RunExecutionLeaseStatus = "released"
)

func (s RunExecutionLeaseStatus) Valid() bool {
	return s == RunExecutionLeaseActive || s == RunExecutionLeaseReleased
}

type RunExecutionLease struct {
	RunID      string
	LeaseID    string
	OwnerID    string
	Generation int64
	Status     RunExecutionLeaseStatus
	AcquiredAt time.Time
	RenewedAt  time.Time
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (l RunExecutionLease) Validate() error {
	for label, value := range map[string]string{
		"run id": l.RunID, "lease id": l.LeaseID, "owner id": l.OwnerID,
	} {
		if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
			len([]rune(value)) > MaxRunLeaseIdentityRunes {
			return fmt.Errorf("run execution lease %s is required, normalized, and bounded", label)
		}
	}
	if l.Generation <= 0 {
		return errors.New("run execution lease generation must be positive")
	}
	if !l.Status.Valid() {
		return fmt.Errorf("invalid run execution lease status %q", l.Status)
	}
	if l.AcquiredAt.IsZero() || l.RenewedAt.IsZero() || l.ExpiresAt.IsZero() ||
		l.RenewedAt.Before(l.AcquiredAt) || !l.ExpiresAt.After(l.RenewedAt) {
		return errors.New("run execution lease timestamps are invalid")
	}
	if l.Status == RunExecutionLeaseActive && l.ReleasedAt != nil {
		return errors.New("active run execution lease cannot have a release time")
	}
	if l.Status == RunExecutionLeaseReleased &&
		(l.ReleasedAt == nil || l.ReleasedAt.IsZero() || l.ReleasedAt.Before(l.AcquiredAt)) {
		return errors.New("released run execution lease requires a valid release time")
	}
	return nil
}

func (l RunExecutionLease) ActiveAt(at time.Time) bool {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return l.Status == RunExecutionLeaseActive && at.UTC().Before(l.ExpiresAt)
}

type AcquireRunExecutionLeaseRequest struct {
	RunID   string
	OwnerID string
	LeaseID string
	TTL     time.Duration
}

func (r AcquireRunExecutionLeaseRequest) Normalize() (AcquireRunExecutionLeaseRequest, error) {
	r.RunID = strings.TrimSpace(r.RunID)
	r.OwnerID = strings.TrimSpace(r.OwnerID)
	r.LeaseID = strings.TrimSpace(r.LeaseID)
	for label, value := range map[string]string{"run id": r.RunID, "owner id": r.OwnerID} {
		if value == "" || !utf8.ValidString(value) || len([]rune(value)) > MaxRunLeaseIdentityRunes {
			return AcquireRunExecutionLeaseRequest{},
				fmt.Errorf("run execution lease %s is required and bounded", label)
		}
	}
	if !utf8.ValidString(r.LeaseID) || len([]rune(r.LeaseID)) > MaxRunLeaseIdentityRunes {
		return AcquireRunExecutionLeaseRequest{}, errors.New("run execution lease replay id must be bounded UTF-8")
	}
	if err := ValidateRunExecutionLeaseTTL(r.TTL); err != nil {
		return AcquireRunExecutionLeaseRequest{}, err
	}
	return r, nil
}

func ValidateRunExecutionLeaseTTL(ttl time.Duration) error {
	if ttl < MinRunExecutionLeaseTTL || ttl > MaxRunExecutionLeaseTTL {
		return fmt.Errorf("run execution lease TTL must be between %s and %s",
			MinRunExecutionLeaseTTL, MaxRunExecutionLeaseTTL)
	}
	return nil
}

type RunExecutionLeaseAcquisition struct {
	Lease    RunExecutionLease
	Replayed bool
	TookOver bool
}
