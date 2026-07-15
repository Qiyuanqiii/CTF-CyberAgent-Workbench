//go:build !windows

package sandbox

import (
	"context"
	"net"
	"net/http"
	"time"
)

type localDockerContainerWriteTransport struct {
	inner dockerEngineContainerWriteTransport
}

func (transport localDockerContainerWriteTransport) Endpoint() DockerObservationEndpoint {
	return transport.inner.Endpoint()
}

func (transport localDockerContainerWriteTransport) Rehearse(ctx context.Context,
	request DockerContainerWriteRequest,
) (DockerContainerWriteResult, error) {
	return transport.inner.Rehearse(ctx, request)
}

func (transport localDockerContainerWriteTransport) Stage(ctx context.Context,
	request DockerContainerWriteRequest,
) (DockerContainerStageResult, error) {
	return transport.inner.Stage(ctx, request)
}

func (transport localDockerContainerWriteTransport) Cleanup(ctx context.Context,
	request DockerContainerWriteRequest, stage DockerContainerStageResult,
) (DockerContainerCleanupResult, error) {
	return transport.inner.Cleanup(ctx, request, stage)
}

func NewLocalDockerContainerWriteTransport() DockerContainerWriteTransport {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: -1}
	httpTransport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", localDockerUnixSocket)
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: httpTransport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	transport, err := newDockerEngineContainerWriteTransport(client, endpoint)
	if err != nil {
		return newUnsupportedDockerContainerWriteTransport()
	}
	return localDockerContainerWriteTransport{inner: transport}
}
