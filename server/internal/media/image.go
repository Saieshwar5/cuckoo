package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	// The formats a phone and a browser actually produce. Registering them
	// is what lets image.Decode recognise them.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
)

// errNotDecodable means the bytes are a picture this server cannot read —
// HEIC from an iPhone, WebP from a browser. It is not a failure: the file
// is stored and sent as it is, without a small copy, and the app shows it
// if the phone can.
var errNotDecodable = errors.New("media: unsupported image format")

// measure reads a picture's dimensions from its header, without decoding
// the pixels.
func measure(data []byte) (int32, int32, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, errNotDecodable
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, errNotDecodable
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return 0, 0, fmt.Errorf("%w: %d by %d pixels", errTooManyPixels, cfg.Width, cfg.Height)
	}
	// Both are positive and below the pixel bound checked above, which is
	// far inside what an int32 holds.
	return int32(cfg.Width), int32(cfg.Height), nil // #nosec G115 -- bounded above
}

var errTooManyPixels = errors.New("media: picture is too large to decode")

// thumbnail makes the small copy the app draws in a bubble: JPEG, because
// every phone decodes one quickly and a preview does not need transparency
// or another copy of the original's size.
func thumbnail(data []byte, w, h int32) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errNotDecodable
	}
	tw, th := fit(int(w), int(h), thumbMaxPixels)
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	// CatmullRom costs more than the bilinear scalers and looks it: this
	// runs once per picture, and the result is looked at many times.
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, fmt.Errorf("media: encode thumbnail: %w", err)
	}
	return out.Bytes(), nil
}

// fit scales a rectangle down so its long side is max, keeping its shape.
// A picture already smaller than that is left alone: enlarging it would
// cost bytes and add nothing.
func fit(w, h, max int) (int, int) {
	if w <= max && h <= max {
		return w, h
	}
	if w >= h {
		return max, atLeastOne(h * max / w)
	}
	return atLeastOne(w * max / h), max
}

func atLeastOne(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
