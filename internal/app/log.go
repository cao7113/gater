package app

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type LogBuffer struct {
	mu       sync.RWMutex
	lines    []string
	maxLines int
}

func NewLogBuffer(maxLines int) *LogBuffer {
	return &LogBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

func (b *LogBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := string(p)
	rawLines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, line := range rawLines {
		if i == len(rawLines)-1 && line == "" {
			continue
		}
		if len(b.lines) >= b.maxLines {
			b.lines = b.lines[1:]
		}
		b.lines = append(b.lines, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line))
	}
	return len(p), nil
}

func (b *LogBuffer) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return strings.Join(b.lines, "\n")
}

func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = b.lines[:0]
}
