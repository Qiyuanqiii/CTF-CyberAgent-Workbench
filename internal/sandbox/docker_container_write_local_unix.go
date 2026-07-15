//go:build !windows

package sandbox

import (
	"context"
	"net"
	"net/http"
	"time"
)

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
	return transport
}
