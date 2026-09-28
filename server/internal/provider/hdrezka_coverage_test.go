package provider_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestCovHdrezkaDecodeStreamURLTable(t *testing.T) {
	original := "[1080p]https://cdn.stream.com/video_1080.m3u8,[720p]https://cdn.stream.com/video_720.m3u8"
	encoded := base64.StdEncoding.EncodeToString([]byte(original))
	trashes := []string{"$$#!!@#!@##", "_@#@_#@_###", "@@@@@!#!@#"}

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "no prefix passthrough", input: original, want: original},
		{name: "empty passthrough", input: "", want: ""},
		{name: "valid base64 with prefix", input: "#h" + encoded, want: original},
		{name: "invalid base64", input: "#h!!!not-base64!!!", wantErr: true},
		{name: "prefix only", input: "#h", want: ""},
	}

	// Кожен trash-токен окремо всередині base64.
	for _, tr := range trashes {
		mid := len(encoded) / 2
		cases = append(cases, struct {
			name    string
			input   string
			want    string
			wantErr bool
		}{name: "trash in middle " + tr, input: "#h" + encoded[:mid] + tr + encoded[mid:], want: original})
	}

	// Токени на позиції 0 і в кінці.
	cases = append(cases, struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{name: "trash at start", input: "#h" + trashes[0] + encoded, want: original})
	cases = append(cases, struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{name: "trash at end", input: "#h" + encoded + trashes[2], want: original})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := provider.DecodeStreamURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCovHdrezkaGetStreamsIsPure(t *testing.T) {
	p := provider.NewHdrezkaProvider(nil)

	const itemURL = "https://x/y"
	resp, err := p.GetStreams(context.Background(), itemURL, 0, 0, "")
	if err != nil {
		t.Fatalf("GetStreams failed: %v", err)
	}
	if resp.ProviderID != "hdrezka" {
		t.Errorf("expected ProviderID hdrezka, got %q", resp.ProviderID)
	}
	if len(resp.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(resp.Streams))
	}
	s := resp.Streams[0]
	if s.Quality != "Auto / 1080p" {
		t.Errorf("expected Quality %q, got %q", "Auto / 1080p", s.Quality)
	}
	if s.URL != itemURL || s.DirectURL != itemURL {
		t.Errorf("expected URL==DirectURL==%q, got URL=%q DirectURL=%q", itemURL, s.URL, s.DirectURL)
	}
	if s.Headers["Referer"] != "https://hdrezka.me/" {
		t.Errorf("expected Referer %q, got %q", "https://hdrezka.me/", s.Headers["Referer"])
	}
}

func TestCovHdrezkaIdentity(t *testing.T) {
	p := provider.NewHdrezkaProvider(nil)
	if p.ID() != "hdrezka" {
		t.Errorf("expected ID hdrezka, got %q", p.ID())
	}
	if p.Name() != "HDRezka" {
		t.Errorf("expected Name HDRezka, got %q", p.Name())
	}
	if p.BaseURL() != "https://hdrezka.me" {
		t.Errorf("expected BaseURL https://hdrezka.me, got %q", p.BaseURL())
	}
}

func TestCovEneyidaIdentity(t *testing.T) {
	p := provider.NewEneyidaProvider(nil)
	if p.ID() != "eneyida" {
		t.Errorf("expected ID eneyida, got %q", p.ID())
	}
	if p.Name() != "Eneyida" {
		t.Errorf("expected Name Eneyida, got %q", p.Name())
	}
	if p.BaseURL() != "https://eneyida.tv" {
		t.Errorf("expected BaseURL https://eneyida.tv, got %q", p.BaseURL())
	}
}
