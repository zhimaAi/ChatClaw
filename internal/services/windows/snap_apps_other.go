//go:build !windows && !darwin

package windows

func listRunningApps() ([]SnapAppCandidate, error) {
	return []SnapAppCandidate{}, nil
}
