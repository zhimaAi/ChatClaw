//go:build !windows && !darwin

package winsnap

// SendTextToTarget is not supported on this platform.
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string) error {
	return ErrNotSupported
}

// PasteTextToTarget is not supported on this platform.
func PasteTextToTarget(targetProcess string, text string) error {
	return ErrNotSupported
}
