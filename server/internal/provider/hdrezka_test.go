package provider_test

import (
	"encoding/base64"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestHDRezkaDecodeStreamURL(t *testing.T) {
	originalStream := "[1080p]https://cdn.stream.com/video_1080.m3u8,[720p]https://cdn.stream.com/video_720.m3u8"
	encodedBase64 := base64.StdEncoding.EncodeToString([]byte(originalStream))

	// Імітація додавання сміттєвих токенів Rezka
	garbageEncoded := "#h" + encodedBase64[:10] + "$$#!!@#!@##" + encodedBase64[10:20] + "_@#@_#@_###" + encodedBase64[20:]

	decoded, err := provider.DecodeStreamURL(garbageEncoded)
	if err != nil {
		t.Fatalf("failed to decode rezka stream: %v", err)
	}

	if decoded != originalStream {
		t.Errorf("expected '%s', got '%s'", originalStream, decoded)
	}
}
