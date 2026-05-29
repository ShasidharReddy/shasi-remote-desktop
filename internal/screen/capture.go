package screen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
)

type ScreenCapture struct {
	FPS        int
	Quality    int
	LastWidth  int
	LastHeight int
}

func NewScreenCapture(fps, quality int) *ScreenCapture {
	return &ScreenCapture{
		FPS:     fps,
		Quality: quality,
	}
}

func (sc *ScreenCapture) CaptureFrame() (*protocol.ScreenFramePayload, error) {
	img := createPlaceholderImage(1920, 1080)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: sc.Quality}); err != nil {
		return nil, fmt.Errorf("encode error: %w", err)
	}

	payload := &protocol.ScreenFramePayload{
		Width:  img.Bounds().Dx(),
		Height: img.Bounds().Dy(),
		Data:   buf.Bytes(),
	}

	sc.LastWidth = payload.Width
	sc.LastHeight = payload.Height

	return payload, nil
}

func (sc *ScreenCapture) DecodeFrame(data []byte) (image.Image, error) {
	return jpeg.Decode(bytes.NewReader(data))
}

func (sc *ScreenCapture) FrameToJSON(frame *protocol.ScreenFramePayload) (json.RawMessage, error) {
	return json.Marshal(frame)
}

func createPlaceholderImage(width, height int) image.Image {
	bounds := image.Rect(0, 0, width, height)
	img := image.NewRGBA(bounds)

	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, blue)
		}
	}

	return img
}
