//go:build !windows

package sandbox

import (
	"context"
	"net"
	"net/http"
	"time"
)

type localDockerRuntimeInputApplicationTransport struct {
	inner dockerEngineContainerWriteTransport
}

func (transport localDockerRuntimeInputApplicationTransport) Endpoint() DockerObservationEndpoint {
	return transport.inner.Endpoint()
}

func (transport localDockerRuntimeInputApplicationTransport) Apply(ctx context.Context,
	intent DockerRuntimeInputApplicationIntent, lease DockerRuntimeInputApplicationLease,
	request DockerRuntimeInputApplicationRequest,
) (DockerRuntimeInputApplicationResult, error) {
	return transport.inner.Apply(ctx, intent, lease, request)
}

func NewLocalDockerRuntimeInputApplicationTransport() DockerRuntimeInputApplicationTransport {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: -1}
	httpTransport := &http.Transport{Proxy: nil, DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", localDockerUnixSocket)
		}}
	client := &http.Client{Transport: httpTransport, Timeout: 2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	transport, err := newDockerEngineContainerWriteTransport(client, endpoint)
	if err != nil {
		return newUnsupportedDockerRuntimeInputApplicationTransport()
	}
	return localDockerRuntimeInputApplicationTransport{inner: transport}
}
