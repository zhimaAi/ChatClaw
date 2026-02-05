//go:build !windows && !darwin

package winsnap

// SendTextToTarget is not supported on this platform.
// noClick and clickOffsetX/Y are ignored on this platform.
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string, noClick bool, clickOffsetX, clickOffsetY int) error {
	return ErrNotSupported
}

// PasteTextToTarget is not supported on this platform.
// noClick and clickOffsetX/Y are ignored on this platform.
func PasteTextToTarget(targetProcess string, text string, noClick bool, clickOffsetX, clickOffsetY int) error {
	return ErrNotSupported
}
