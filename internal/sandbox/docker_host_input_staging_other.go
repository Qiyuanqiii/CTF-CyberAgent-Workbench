//go:build !linux

package sandbox

func NewLocalDockerHostInputStager() DockerHostInputStager {
	return UnavailableDockerHostInputStager{code: DockerHostInputStagingErrorUnsupported}
}
