package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Ensures that the Out writer gets a line at a time. Useful if you have multiple things
// writing in parallel.
type lineBuffer struct {
	Out    io.Writer
	Prefix []byte // Optional

	curr bytes.Buffer
	mu   sync.Mutex
}

func (lb *lineBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for _, b := range p {
		lb.curr.WriteByte(b)
		// line ending
		if b == '\n' {
			line := lb.curr.Bytes()
			lb.curr.Reset()
			if len(lb.Prefix) > 0 {
				line = append(lb.Prefix[:], line[:]...)
			}
			n, err := lb.Out.Write(line)
			if err != nil {
				return n, fmt.Errorf("out err: %w", err)
			}
		}
	}
	return len(p), nil
}

// Writes any remainder out as a line
func (lb *lineBuffer) Flush() {
	if lb.curr.Len() > 0 {
		lb.Out.Write(lb.curr.Bytes())
		lb.curr.Reset()
	}
}

type linesWriter struct {
	lines []string
	curr  strings.Builder
	mu    sync.Mutex
}

func (lw *linesWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	for _, b := range p {
		if b == '\n' {
			// line ending
			line := lw.curr.String()
			lw.curr.Reset()
			if len(line) == 0 {
				continue
			}
			lw.lines = append(lw.lines, line)
		} else {
			// normal byte
			lw.curr.WriteByte(b)
		}
	}
	return len(p), nil
}

// Should only be called after all writing is done
func (lw *linesWriter) Lines() []string {
	// Write any remaining bytes
	if lw.curr.Len() > 0 {
		lw.lines = append(lw.lines, lw.curr.String())
	}

	return lw.lines
}
