//go:build windows

package sandbox

func NewLocalDockerRuntimeInputApplicationTransport() DockerRuntimeInputApplicationTransport {
	return newUnsupportedDockerRuntimeInputApplicationTransport()
}
