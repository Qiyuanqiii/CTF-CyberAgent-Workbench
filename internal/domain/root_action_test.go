package domain

import (
	"strings"
	"testing"
)

func TestRootActionValidate(t *testing.T) {
	tests := []struct {
		name    string
		action  RootAction
		wantErr bool
	}{
		{name: "continue", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionContinue, Message: "keep planning"}},
		{name: "finish", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionFinish, Message: "done", Summary: "review complete"}},
		{name: "wait", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionWait, Message: "waiting", Reason: "user input required"}},
		{name: "unknown", action: RootAction{Version: RootLifecycleVersion, Kind: "launch", Message: "no"}, wantErr: true},
		{name: "finish without summary", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionFinish, Message: "done"}, wantErr: true},
		{name: "wait with summary", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionWait, Message: "waiting", Summary: "not done", Reason: "input"}, wantErr: true},
		{name: "continue with reason", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionContinue, Message: "continue", Reason: "extra"}, wantErr: true},
		{name: "oversized kind", action: RootAction{Version: RootLifecycleVersion, Kind: RootActionKind(strings.Repeat("x", 33)), Message: "no"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
