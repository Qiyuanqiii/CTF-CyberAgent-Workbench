//go:build !windows

package sandbox

import (
	"context"
	"net"
	"net/http"
	"time"
)

type localDockerRuntimeInputResourceInspector struct {
	inner dockerEngineContainerWriteTransport
}

func (transport localDockerRuntimeInputResourceInspector) Endpoint() DockerObservationEndpoint {
	return transport.inner.Endpoint()
}

func (transport localDockerRuntimeInputResourceInspector) Inspect(ctx context.Context,
	descriptor DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceObservation, error) {
	return transport.inner.inspectRuntimeInputResources(ctx, descriptor)
}

type localDockerRuntimeInputResourceCleanupTransport struct {
	inner dockerEngineContainerWriteTransport
}

func (transport localDockerRuntimeInputResourceCleanupTransport) Endpoint() DockerObservationEndpoint {
	return transport.inner.Endpoint()
}

func (transport localDockerRuntimeInputResourceCleanupTransport) Cleanup(ctx context.Context,
	intent DockerRuntimeInputResourceCleanupIntent, lease DockerRuntimeInputResourceCleanupLease,
	descriptor DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceCleanupResult, error) {
	return transport.inner.cleanupRuntimeInputResources(ctx, intent, lease, descriptor)
}

func newLocalDockerRuntimeInputResourceTransportCore() (dockerEngineContainerWriteTransport, error) {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: -1}
	httpTransport := &http.Transport{Proxy: nil, DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", localDockerUnixSocket)
		}}
	client := &http.Client{Transport: httpTransport, Timeout: 2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return newDockerEngineContainerWriteTransport(client, endpoint)
}

func NewLocalDockerRuntimeInputResourceInspector() DockerRuntimeInputResourceInspector {
	transport, err := newLocalDockerRuntimeInputResourceTransportCore()
	if err != nil {
		return newUnsupportedDockerRuntimeInputResourceTransport()
	}
	return localDockerRuntimeInputResourceInspector{inner: transport}
}

func NewLocalDockerRuntimeInputResourceCleanupTransport() DockerRuntimeInputResourceCleanupTransport {
	transport, err := newLocalDockerRuntimeInputResourceTransportCore()
	if err != nil {
		return newUnsupportedDockerRuntimeInputResourceTransport()
	}
	return localDockerRuntimeInputResourceCleanupTransport{inner: transport}
}
