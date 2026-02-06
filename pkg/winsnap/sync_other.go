//go:build !windows

package winsnap

// SyncRightOfProcessNow is not supported on non-Windows platforms.
// The snapping implementations on other platforms handle restore/resync differently.
func SyncRightOfProcessNow(_ AttachOptions) error {
	return ErrNotSupported
}
