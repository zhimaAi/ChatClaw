//go:build darwin && !cgo

package winsnap

import "errors"

func attachRightOfProcess(_ AttachOptions) (Controller, error) {
	return nil, errors.New("winsnap: not supported without cgo on darwin")
}
