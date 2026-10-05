package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/logging"
)

// TestNewFormats перевіряє, що format=json дає JSON, а решта — текст.
func TestNewFormats(t *testing.T) {
	jsonBuf := &bytes.Buffer{}
	logging.NewHandler("info", "json", jsonBuf).Handle(nil, slog.NewRecord(time.Time{}, slog.LevelInfo, "hello", 0))
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(jsonBuf.Bytes()), &rec); err != nil {
		t.Fatalf("json-формат не розобраний: %v (%s)", err, jsonBuf.String())
	}
	if rec["msg"] != "hello" || rec["level"] != "INFO" {
		t.Errorf("неочікуваний запис: %v", rec)
	}

	textBuf := &bytes.Buffer{}
	logging.NewHandler("info", "text", textBuf).Handle(nil, slog.NewRecord(time.Time{}, slog.LevelInfo, "hello", 0))
	if !strings.Contains(textBuf.String(), "msg=hello") {
		t.Errorf("text-формат не розобраний: %s", textBuf.String())
	}

	unknownBuf := &bytes.Buffer{}
	logging.NewHandler("info", "xml", unknownBuf).Handle(nil, slog.NewRecord(time.Time{}, slog.LevelInfo, "hello", 0))
	if strings.HasPrefix(strings.TrimSpace(unknownBuf.String()), "{") {
		t.Errorf("невідомий формат має давати text, отримано %s", unknownBuf.String())
	}
}

// TestParseLevel перевіряє мапу рівнів та дефолт info.
func TestParseLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"INFO":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"chatty":  slog.LevelInfo,
	}
	for in, want := range tests {
		if got := logging.ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, очікувалося %v", in, got, want)
		}
	}
}

// TestLevelFiltering перевіряє, що нижчі рівні відкидаються.
func TestLevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	l := slog.New(logging.NewHandler("warn", "text", buf))
	l.Info("dropped")
	if buf.Len() != 0 {
		t.Errorf("INFO має бути відкинутий при level=warn: %s", buf.String())
	}
	l.Warn("kept")
	if !strings.Contains(buf.String(), "kept") {
		t.Errorf("WARN має бути записаний: %s", buf.String())
	}
}

// TestSetDefaultAndL перевіряє підміну логгера (так само, як у middleware-тестах).
func TestSetDefaultAndL(t *testing.T) {
	prev := logging.L()
	t.Cleanup(func() { logging.SetDefault(prev) })

	if prev == nil {
		t.Fatal("L() повернув nil")
	}

	buf := &bytes.Buffer{}
	replacement := slog.New(logging.NewHandler("debug", "json", buf))
	logging.SetDefault(replacement)

	if logging.L() != replacement {
		t.Error("L() має повертати встановлений логгер")
	}
	if slog.Default() != replacement {
		t.Error("slog.SetDefault не викликано")
	}

	logging.L().Info("hello", "path", "/healthz")
	if !strings.Contains(buf.String(), `"path":"/healthz"`) {
		t.Errorf("лог не потрапив у буфер: %s", buf.String())
	}

	logging.SetDefault(nil) // не має панікувати і не має змінювати поточний
	if logging.L() != replacement {
		t.Error("SetDefault(nil) змінив логгер")
	}
}

// TestInitInstallsLogger перевіряє комбіновану ініціалізацію з конфігурації.
func TestInitInstallsLogger(t *testing.T) {
	prev := logging.L()
	t.Cleanup(func() { logging.SetDefault(prev) })

	l := logging.Init("error", "json")
	if l == nil || l.Enabled(nil, slog.LevelInfo) {
		t.Error("очікувався логгер з рівнем error")
	}
	if !l.Enabled(nil, slog.LevelError) {
		t.Error("помилки мають логуватися")
	}
}
