//go:build !windows && !darwin

package winsnap

import "errors"

var ErrNotSupported = errors.New("winsnap: not supported on this platform yet")

func attachRightOfProcess(_ AttachOptions) (Controller, error) {
	return nil, ErrNotSupported
}
