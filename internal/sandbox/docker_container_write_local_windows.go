//go:build windows

package sandbox

func NewLocalDockerContainerWriteTransport() DockerContainerWriteTransport {
	return newUnsupportedDockerContainerWriteTransport()
}
