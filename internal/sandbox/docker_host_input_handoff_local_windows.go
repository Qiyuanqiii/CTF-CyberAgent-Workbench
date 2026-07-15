//go:build windows

package sandbox

func NewLocalDockerHostInputHandoffTransport() DockerHostInputHandoffTransport {
	return newUnsupportedDockerHostInputHandoffTransport()
}
