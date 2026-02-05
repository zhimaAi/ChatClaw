//go:build darwin && cgo

package thumbnail

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa -framework QuickLook -framework CoreServices

#import <Cocoa/Cocoa.h>
#import <QuickLook/QuickLook.h>

// Generate thumbnail using Quick Look framework
// Returns PNG data, caller must free with free()
// size: maximum dimension (width or height)
// outLen: output length of PNG data
static void* generateThumbnail(const char* path, int size, int* outLen) {
    @autoreleasepool {
        *outLen = 0;

        NSString *filePath = [NSString stringWithUTF8String:path];
        NSURL *fileURL = [NSURL fileURLWithPath:filePath];

        // Request thumbnail using Quick Look
        CGSize maxSize = CGSizeMake(size, size);
        NSDictionary *options = @{
            (NSString *)kQLThumbnailOptionIconModeKey: @NO,
        };

        CGImageRef thumbnail = QLThumbnailImageCreate(
            kCFAllocatorDefault,
            (__bridge CFURLRef)fileURL,
            maxSize,
            (__bridge CFDictionaryRef)options
        );

        if (thumbnail == NULL) {
            return NULL;
        }

        // Convert to NSImage, then to PNG data
        NSBitmapImageRep *bitmapRep = [[NSBitmapImageRep alloc] initWithCGImage:thumbnail];
        CGImageRelease(thumbnail);

        if (bitmapRep == nil) {
            return NULL;
        }

        NSData *pngData = [bitmapRep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        if (pngData == nil || pngData.length == 0) {
            return NULL;
        }

        // Copy data to heap (caller must free)
        void *result = malloc(pngData.length);
        if (result == NULL) {
            return NULL;
        }
        memcpy(result, pngData.bytes, pngData.length);
        *outLen = (int)pngData.length;

        return result;
    }
}

// Get file icon as PNG (fallback when thumbnail is not available)
static void* getFileIcon(const char* path, int size, int* outLen) {
    @autoreleasepool {
        *outLen = 0;

        NSString *filePath = [NSString stringWithUTF8String:path];
        NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:filePath];

        if (icon == nil) {
            return NULL;
        }

        // Set size
        [icon setSize:NSMakeSize(size, size)];

        // Convert to PNG
        NSBitmapImageRep *bitmapRep = [[NSBitmapImageRep alloc]
            initWithBitmapDataPlanes:NULL
            pixelsWide:size
            pixelsHigh:size
            bitsPerSample:8
            samplesPerPixel:4
            hasAlpha:YES
            isPlanar:NO
            colorSpaceName:NSCalibratedRGBColorSpace
            bytesPerRow:0
            bitsPerPixel:0];

        NSGraphicsContext *ctx = [NSGraphicsContext graphicsContextWithBitmapImageRep:bitmapRep];
        [NSGraphicsContext saveGraphicsState];
        [NSGraphicsContext setCurrentContext:ctx];
        [icon drawInRect:NSMakeRect(0, 0, size, size) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1.0];
        [NSGraphicsContext restoreGraphicsState];

        NSData *pngData = [bitmapRep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        if (pngData == nil || pngData.length == 0) {
            return NULL;
        }

        // Copy data to heap (caller must free)
        void *result = malloc(pngData.length);
        if (result == NULL) {
            return NULL;
        }
        memcpy(result, pngData.bytes, pngData.length);
        *outLen = (int)pngData.length;

        return result;
    }
}

// Free allocated memory
static void freeData(void* data) {
    if (data != NULL) {
        free(data);
    }
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// generatePlatformThumbnail generates thumbnail using macOS Quick Look framework
func generatePlatformThumbnail(filePath string, ext string) ([]byte, error) {
	cPath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cPath))

	var outLen C.int

	// Try Quick Look thumbnail first
	data := C.generateThumbnail(cPath, C.int(MaxSize), &outLen)
	if data != nil && outLen > 0 {
		defer C.freeData(data)
		return C.GoBytes(data, outLen), nil
	}

	// Fallback to file icon
	data = C.getFileIcon(cPath, C.int(MaxSize), &outLen)
	if data != nil && outLen > 0 {
		defer C.freeData(data)
		return C.GoBytes(data, outLen), nil
	}

	return nil, errors.New("failed to generate thumbnail")
}
