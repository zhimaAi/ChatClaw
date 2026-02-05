//go:build windows

package thumbnail

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modOle32   = windows.NewLazySystemDLL("ole32.dll")
	modShell32 = windows.NewLazySystemDLL("shell32.dll")
	modGdi32   = windows.NewLazySystemDLL("gdi32.dll")
	modUser32  = windows.NewLazySystemDLL("user32.dll")

	procCoInitializeEx              = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize              = modOle32.NewProc("CoUninitialize")
	procCoCreateInstance            = modOle32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = modShell32.NewProc("SHCreateItemFromParsingName")

	procGetDIBits       = modGdi32.NewProc("GetDIBits")
	procCreateCompatibleDC = modGdi32.NewProc("CreateCompatibleDC")
	procSelectObject    = modGdi32.NewProc("SelectObject")
	procDeleteDC        = modGdi32.NewProc("DeleteDC")
	procDeleteObject    = modGdi32.NewProc("DeleteObject")
	procGetObjectW      = modGdi32.NewProc("GetObjectW")
)

// COM constants
const (
	COINIT_APARTMENTTHREADED = 0x2
	COINIT_DISABLE_OLE1DDE   = 0x4
)

// IShellItemImageFactory GUID
var (
	IID_IShellItemImageFactory = windows.GUID{
		Data1: 0xbcc18b79,
		Data2: 0xba16,
		Data3: 0x442f,
		Data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b},
	}
)

// SIIGBF flags for GetImage
const (
	SIIGBF_RESIZETOFIT   = 0x00000000
	SIIGBF_BIGGERSIZEOK  = 0x00000001
	SIIGBF_MEMORYONLY    = 0x00000002
	SIIGBF_ICONONLY      = 0x00000004
	SIIGBF_THUMBNAILONLY = 0x00000008
	SIIGBF_INCACHEONLY   = 0x00000010
)

// SIZE structure
type SIZE struct {
	CX int32
	CY int32
}

// BITMAP structure
type BITMAP struct {
	BmType       int32
	BmWidth      int32
	BmHeight     int32
	BmWidthBytes int32
	BmPlanes     uint16
	BmBitsPixel  uint16
	BmBits       uintptr
}

// BITMAPINFOHEADER structure
type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

// BITMAPINFO structure
type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

// IShellItemImageFactory interface
type IShellItemImageFactory struct {
	vtbl *IShellItemImageFactoryVtbl
}

type IShellItemImageFactoryVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetImage       uintptr
}

func (v *IShellItemImageFactory) Release() {
	syscall.SyscallN(v.vtbl.Release, uintptr(unsafe.Pointer(v)))
}

func (v *IShellItemImageFactory) GetImage(size SIZE, flags uint32) (windows.Handle, error) {
	var hbmp windows.Handle
	ret, _, _ := syscall.SyscallN(
		v.vtbl.GetImage,
		uintptr(unsafe.Pointer(v)),
		uintptr(*(*int64)(unsafe.Pointer(&size))),
		uintptr(flags),
		uintptr(unsafe.Pointer(&hbmp)),
	)
	if ret != 0 {
		return 0, syscall.Errno(ret)
	}
	return hbmp, nil
}

// generatePlatformThumbnail generates thumbnail using Windows Shell API
func generatePlatformThumbnail(filePath string, ext string) ([]byte, error) {
	// Initialize COM
	ret, _, _ := procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED|COINIT_DISABLE_OLE1DDE)
	if ret != 0 && ret != 1 { // S_OK or S_FALSE
		return nil, errors.New("failed to initialize COM")
	}
	defer procCoUninitialize.Call()

	// Get IShellItemImageFactory
	pathPtr, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return nil, err
	}

	var factory *IShellItemImageFactory
	ret, _, _ = procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&IID_IShellItemImageFactory)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if ret != 0 {
		return nil, errors.New("failed to get shell item")
	}
	defer factory.Release()

	// Get thumbnail (try thumbnail first, then fallback to icon)
	size := SIZE{CX: int32(MaxSize), CY: int32(MaxSize)}
	hbmp, err := factory.GetImage(size, SIIGBF_RESIZETOFIT|SIIGBF_BIGGERSIZEOK)
	if err != nil {
		// Fallback to icon only
		hbmp, err = factory.GetImage(size, SIIGBF_ICONONLY)
		if err != nil {
			return nil, errors.New("failed to get image")
		}
	}
	defer procDeleteObject.Call(uintptr(hbmp))

	// Convert HBITMAP to PNG
	pngData, err := hbitmapToPNG(hbmp)
	if err != nil {
		return nil, err
	}

	return pngData, nil
}

// hbitmapToPNG converts Windows HBITMAP to PNG bytes
func hbitmapToPNG(hbmp windows.Handle) ([]byte, error) {
	// Get bitmap info
	var bmp BITMAP
	ret, _, _ := procGetObjectW.Call(uintptr(hbmp), unsafe.Sizeof(bmp), uintptr(unsafe.Pointer(&bmp)))
	if ret == 0 {
		return nil, errors.New("failed to get bitmap info")
	}

	width := int(bmp.BmWidth)
	height := int(bmp.BmHeight)

	// Create compatible DC
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return nil, errors.New("failed to create DC")
	}
	defer procDeleteDC.Call(hdc)

	// Select bitmap into DC
	procSelectObject.Call(hdc, uintptr(hbmp))

	// Prepare BITMAPINFO
	bi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
			BiWidth:       int32(width),
			BiHeight:      -int32(height), // Top-down DIB
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: 0, // BI_RGB
		},
	}

	// Allocate buffer for pixel data
	pixels := make([]byte, width*height*4)

	// Get bitmap bits
	ret, _, _ = procGetDIBits.Call(
		hdc,
		uintptr(hbmp),
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bi)),
		0, // DIB_RGB_COLORS
	)
	if ret == 0 {
		return nil, errors.New("failed to get bitmap bits")
	}

	// Convert BGRA to RGBA and create image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * 4
			// BGRA -> RGBA
			img.Pix[(y*width+x)*4+0] = pixels[i+2] // R
			img.Pix[(y*width+x)*4+1] = pixels[i+1] // G
			img.Pix[(y*width+x)*4+2] = pixels[i+0] // B
			img.Pix[(y*width+x)*4+3] = pixels[i+3] // A
		}
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
