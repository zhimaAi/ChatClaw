//go:build !darwin && !windows

package thumbnail

import "errors"

// generatePlatformThumbnail is a stub for unsupported platforms
func generatePlatformThumbnail(filePath string, ext string) ([]byte, error) {
	return nil, errors.New("thumbnail generation not supported on this platform")
}
