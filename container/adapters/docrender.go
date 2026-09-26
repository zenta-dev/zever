package adapters

import (
	"github.com/zenta-dev/zever/document"
	documentlocal "github.com/zenta-dev/zever/document/local"
	"github.com/zenta-dev/zever/media"
	medialocal "github.com/zenta-dev/zever/media/local"
)

// RegisterDoc registers the local rendering adapters the core container no
// longer imports: document/local (headless Chrome via chromedp) and
// media/local (image transforms via bild). The remote/latex document
// adapters stay wired by the container; media has no other adapter.
// Registration only fills factory maps; it performs no I/O. Duplicate
// registrations are ignored, so calling RegisterDoc more than once (or
// alongside RegisterAll) is safe.
//
// Note both facades default to `local` in config.Default, so a default
// config resolves document/media only after RegisterDoc (or RegisterAll).
func RegisterDoc() {
	_ = document.Register(document.Local, documentlocal.New)

	_ = media.Register(media.Local, medialocal.New)
}
