//go:build !windows

package browserruntime

func defaultBrowserDiscoveryRoots() []DiscoveryRoot {
	return nil
}
