package platform

import (
	"os"
	"sync/atomic"
)

var ImageMimeTypes = []string{
	"image/jpeg",
	"image/png",
	"image/gif",
	"image/bmp",
	"image/x-ms-bmp",
	"image/webp",
	"image/tiff",
	"image/svg+xml",
	"image/x-icon",
	"image/vnd.microsoft.icon",
	"image/x-win-bitmap",
	"image/apng",
	"image/avif",
	"image/avif-sequence",
	"image/heic",
	"image/heic-sequence",
	"image/heif",
	"image/heif-sequence",
	"image/jxl",
	"image/x-canon-cr2",
	"image/x-canon-cr3",
	"image/x-nikon-nef",
	"image/x-sony-arw",
	"image/x-adobe-dng",
	"image/x-fuji-raf",
	"image/x-olympus-orf",
	"image/x-panasonic-rw2",
	"image/x-pentax-pef",
}

const DefaultDesktopID = "pixgo.desktop"

var desktopID atomic.Value

func SetDesktopID(id string) { desktopID.Store(id) }

func DesktopID() string {
	if id := os.Getenv("PIXGO_DESKTOP_ID"); id != "" {
		return id
	}
	if id, ok := desktopID.Load().(string); ok && id != "" {
		return id
	}
	return DefaultDesktopID
}
