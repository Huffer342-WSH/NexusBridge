// cover_images.go 负责将远程封面转换为适合本地缓存的 WebP 图片。
package core

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/skrashevich/go-webp"
	"golang.org/x/image/draw"
)

const (
	maxCoverSourceBytes   = 50 << 20
	maxCoverDecodedPixels = 64 * 1024 * 1024
	maxCoverHeight        = 1440
	coverWebPQuality      = 82
)

// encodeCoverForCache 解码远程原图，必要时等比缩小，并编码为 WebP 缓存数据。
func encodeCoverForCache(source []byte) ([]byte, error) {
	if len(source) == 0 || len(source) > maxCoverSourceBytes {
		return nil, fmt.Errorf("invalid cover source size: %d", len(source))
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("decode cover image config: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxCoverDecodedPixels {
		return nil, fmt.Errorf("invalid cover dimensions: %dx%d", config.Width, config.Height)
	}

	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("decode cover image: %w", err)
	}
	converted := decoded
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid decoded cover dimensions: %dx%d", width, height)
	}
	if height > maxCoverHeight {
		targetWidth := int((int64(width)*maxCoverHeight + int64(height)/2) / int64(height))
		if targetWidth < 1 {
			targetWidth = 1
		}
		resized := image.NewNRGBA(image.Rect(0, 0, targetWidth, maxCoverHeight))
		draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, bounds, draw.Over, nil)
		converted = resized
	}

	var encoded bytes.Buffer
	if err := webp.Encode(&encoded, converted, &webp.Options{
		Lossy:   true,
		Quality: coverWebPQuality,
	}); err != nil {
		return nil, fmt.Errorf("encode cover as webp: %w", err)
	}
	if encoded.Len() == 0 {
		return nil, fmt.Errorf("encode cover as webp: empty result")
	}
	return encoded.Bytes(), nil
}
