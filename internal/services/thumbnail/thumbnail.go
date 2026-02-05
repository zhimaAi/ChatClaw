// Package thumbnail provides file thumbnail generation for different platforms.
package thumbnail

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

// MaxSize is the maximum thumbnail dimension (256x256)
const MaxSize = 256

// Result represents the thumbnail generation result
type Result struct {
	// Base64 encoded PNG image with data URI prefix (e.g., "data:image/png;base64,...")
	// Empty string if generation failed
	DataURI string
	// Error message if generation failed
	Error string
}

// Generate generates a thumbnail for the given file path.
// Returns a Result containing the base64 encoded PNG data URI or an error message.
func Generate(filePath string) Result {
	// Check file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return Result{Error: "file not found"}
	}

	// Get file extension
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), "."))

	// Try platform-specific thumbnail generation
	imgData, err := generatePlatformThumbnail(filePath, ext)
	if err != nil {
		return Result{Error: err.Error()}
	}

	if len(imgData) == 0 {
		return Result{Error: "no thumbnail data generated"}
	}

	// Encode to base64 with data URI prefix
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgData)
	return Result{DataURI: dataURI}
}
