//go:build darwin && !cgo

package thumbnail

import "errors"

// generatePlatformThumbnail is a stub when CGO is disabled on macOS
func generatePlatformThumbnail(filePath string, ext string) ([]byte, error) {
	return nil, errors.New("thumbnail generation requires CGO on macOS")
}
