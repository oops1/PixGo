package platform

import "os"

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
}

const DefaultDesktopID = "pixgo.desktop"

func DesktopID() string {
	if id := os.Getenv("PIXGO_DESKTOP_ID"); id != "" {
		return id
	}
	return DefaultDesktopID
}
