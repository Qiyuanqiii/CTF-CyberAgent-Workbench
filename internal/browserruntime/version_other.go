//go:build !windows

package browserruntime

func browserExecutableVersion(string) (string, ExecutableVersionSource, bool) {
	return "", VersionSourceUnavailable, false
}
