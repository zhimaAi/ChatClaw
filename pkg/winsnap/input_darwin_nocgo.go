//go:build darwin && !cgo

package winsnap

import "errors"

// SendTextToTarget is not supported without CGO on macOS.
// noClick and clickOffsetX/Y are ignored on this platform.
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string, noClick bool, clickOffsetX, clickOffsetY int) error {
	return errors.New("winsnap: SendTextToTarget requires cgo on darwin")
}

// PasteTextToTarget is not supported without CGO on macOS.
// noClick and clickOffsetX/Y are ignored on this platform.
func PasteTextToTarget(targetProcess string, text string, noClick bool, clickOffsetX, clickOffsetY int) error {
	return errors.New("winsnap: PasteTextToTarget requires cgo on darwin")
}
