//go:build !windows && !darwin

package winsnap

func attachRightOfProcess(_ AttachOptions) (Controller, error) {
	return nil, ErrNotSupported
}
