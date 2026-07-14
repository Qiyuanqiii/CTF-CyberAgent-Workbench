//go:build windows

package sandbox

func NewLocalDockerReadOnlyTransport() DockerReadOnlyTransport {
	return NewUnavailableDockerReadOnlyTransport(DockerObservationEndpointLocalNPipe,
		DockerObservationFailureTransportUnsupported)
}
