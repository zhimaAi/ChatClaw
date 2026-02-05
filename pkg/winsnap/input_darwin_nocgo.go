//go:build darwin && !cgo

package winsnap

import "errors"

// SendTextToTarget is not supported without CGO on macOS.
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string) error {
	return errors.New("winsnap: SendTextToTarget requires cgo on darwin")
}

// PasteTextToTarget is not supported without CGO on macOS.
func PasteTextToTarget(targetProcess string, text string) error {
	return errors.New("winsnap: PasteTextToTarget requires cgo on darwin")
}
