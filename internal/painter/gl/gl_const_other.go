//go:build !darwin
// +build !darwin

package gl

import (
	"fyne.io/fyne/v2/internal/driver/mobile/gl"
)

const (
	singleChannelColorFormat = gl.LUMINANCE
)
