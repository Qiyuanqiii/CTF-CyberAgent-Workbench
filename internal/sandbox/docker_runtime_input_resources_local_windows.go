//go:build windows

package sandbox

import "context"

type unsupportedLocalDockerRuntimeInputResourceInspector struct {
	inner UnavailableDockerRuntimeInputResourceTransport
}

func (value unsupportedLocalDockerRuntimeInputResourceInspector) Endpoint() DockerObservationEndpoint {
	return value.inner.Endpoint()
}

func (value unsupportedLocalDockerRuntimeInputResourceInspector) Inspect(ctx context.Context,
	descriptor DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceObservation, error) {
	return value.inner.Inspect(ctx, descriptor)
}

type unsupportedLocalDockerRuntimeInputResourceCleanupTransport struct {
	inner UnavailableDockerRuntimeInputResourceTransport
}

func (value unsupportedLocalDockerRuntimeInputResourceCleanupTransport) Endpoint() DockerObservationEndpoint {
	return value.inner.Endpoint()
}

func (value unsupportedLocalDockerRuntimeInputResourceCleanupTransport) Cleanup(ctx context.Context,
	intent DockerRuntimeInputResourceCleanupIntent, lease DockerRuntimeInputResourceCleanupLease,
	descriptor DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceCleanupResult, error) {
	return value.inner.Cleanup(ctx, intent, lease, descriptor)
}

func NewLocalDockerRuntimeInputResourceInspector() DockerRuntimeInputResourceInspector {
	return unsupportedLocalDockerRuntimeInputResourceInspector{
		inner: newUnsupportedDockerRuntimeInputResourceTransport(),
	}
}

func NewLocalDockerRuntimeInputResourceCleanupTransport() DockerRuntimeInputResourceCleanupTransport {
	return unsupportedLocalDockerRuntimeInputResourceCleanupTransport{
		inner: newUnsupportedDockerRuntimeInputResourceTransport(),
	}
}
