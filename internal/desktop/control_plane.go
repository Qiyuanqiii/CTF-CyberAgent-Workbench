package desktop

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

// ControlPlane owns the Desktop process' SQLite connection and in-process API.
// It does not listen on a socket and it adds no renderer authority beyond the
// tokens explicitly supplied in ControlPlaneConfig.
type ControlPlane struct {
	stateStore *store.SQLiteStore
	handler    http.Handler
	closeOnce  sync.Once
	closeErr   error
}

type ControlPlaneConfig struct {
	DatabasePath                  string
	ReadToken                     string
	ControlToken                  string
	RunControlEnabled             bool
	RunCreationEnabled            bool
	SessionMessageEnabled         bool
	SessionSteeringControlEnabled bool
	RunLifecycleEnabled           bool
	RunExecutionEnabled           bool
	AppVersion                    string
	UIHandler                     http.Handler
}

func OpenControlPlane(config ControlPlaneConfig) (*ControlPlane, error) {
	if strings.TrimSpace(config.DatabasePath) == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"desktop database path is required")
	}
	stateStore, err := store.Open(config.DatabasePath)
	if err != nil {
		return nil, err
	}
	lifecycleControl := application.NewRunLifecycleControlService(stateStore)
	executionControl := application.NewRunExecutionHandoffService(stateStore,
		llm.NewDefaultRouter(), policy.NewDefaultChecker()).WithActiveCalls(
		application.NewActiveCallRegistry())
	api, err := httpapi.New(stateStore, httpapi.Config{
		AccessToken: config.ReadToken, ControlToken: config.ControlToken,
		RunControlEnabled:             config.RunControlEnabled,
		RunCreationEnabled:            config.RunCreationEnabled,
		SessionMessageEnabled:         config.SessionMessageEnabled,
		SessionSteeringControlEnabled: config.SessionSteeringControlEnabled,
		RunLifecycleEnabled:           config.RunLifecycleEnabled,
		RunExecutionEnabled:           config.RunExecutionEnabled,
		RunLifecycleController:        lifecycleControl,
		RunExecutionController:        executionControl,
		AppVersion:                    config.AppVersion, UIHandler: config.UIHandler,
	})
	if err != nil {
		_ = stateStore.Close()
		return nil, err
	}
	return &ControlPlane{stateStore: stateStore, handler: api.Handler()}, nil
}

func (c *ControlPlane) Handler() http.Handler {
	if c == nil {
		return nil
	}
	return c.handler
}

func (c *ControlPlane) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.stateStore == nil {
			c.closeErr = errors.New("desktop control plane store is unavailable")
			return
		}
		c.closeErr = c.stateStore.Close()
	})
	return c.closeErr
}
