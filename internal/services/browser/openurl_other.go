//go:build !windows

package browser

// openURLWindows is only implemented for Windows. This stub satisfies the compiler
// when building for other platforms (never called due to runtime.GOOS check).
func openURLWindows(_ string) error {
	panic("openURLWindows should only be called on Windows")
}
