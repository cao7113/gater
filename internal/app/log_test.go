package app

import (
	"strings"
	"testing"
)

func TestLogBufferKeepsLatestLines(t *testing.T) {
	buffer := NewLogBuffer(2)
	_, _ = buffer.Write([]byte("first\nsecond\nthird\n"))

	output := buffer.String()
	if strings.Contains(output, "first") {
		t.Fatalf("oldest line should be discarded, got %q", output)
	}
	if !strings.Contains(output, "second") || !strings.Contains(output, "third") {
		t.Fatalf("latest lines missing, got %q", output)
	}
}

func TestLogBufferClear(t *testing.T) {
	buffer := NewLogBuffer(2)
	_, _ = buffer.Write([]byte("line\n"))
	buffer.Clear()

	if got := buffer.String(); got != "" {
		t.Fatalf("cleared buffer = %q, want empty", got)
	}
}
