package kontaktantwort

import (
	"bytes"
	"errors"
	"image"

	"github.com/mdopp/homes/internal/gesichter"
)

// A contact's own photo (owner 2026-11-08). The page is public, so it is
// strict: at most FotoBytes, a JPEG or PNG that decodes whole (the page's
// script turns HEIC and WebP into a JPEG in the browser first: Homes has
// no decoder of its own for them), at most FotoPixel; it is kept as a new
// JPEG of at most FotoKante pixels, made from the pixels alone, so nothing
// of the file (EXIF, the place it was taken, a comment) is kept.
const (
	FotoBytes = 5 << 20
	FotoKante = 512
	FotoPixel = 40_000_000
)

// ErrFoto is a file that is no photo Homes takes.
var ErrFoto = errors.New("kein JPEG- oder PNG-Bild")

// Foto is the uploaded b as it is kept.
func Foto(b []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || len(b) > FotoBytes || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 ||
		cfg.Width > 20000 || cfg.Height > 20000 || cfg.Width*cfg.Height > FotoPixel {
		return nil, ErrFoto
	}
	out, err := gesichter.Klein(b, FotoKante)
	if err != nil {
		return nil, ErrFoto
	}
	return out, nil
}
