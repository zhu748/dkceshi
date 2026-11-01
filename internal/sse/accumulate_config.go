package sse

import "time"

const (
	parsedLineBufferSize = 128
	lineReaderBufferSize = 64 * 1024
)

type AccumulateConfig struct {
	Enabled        bool
	MinChars       int
	MaxWait        time.Duration
	FlushOnFinish  bool
	WordBoundary   bool
	FlushOnNewline bool
}

var productionAccumulate = AccumulateConfig{
	Enabled:        true,
	MinChars:       16,
	MaxWait:        10 * time.Millisecond,
	FlushOnFinish:  true,
	WordBoundary:   false,
	FlushOnNewline: true,
}
