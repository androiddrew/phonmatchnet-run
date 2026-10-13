// Package listen slides a fixed window over a 16-bit PCM stream, scores each
// window and debounces accepted windows into detections.
package listen

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Config sets the window geometry and debouncing, in samples.
type Config struct {
	Window int // samples scored per window
	Hop    int // samples between window ends
	// Confirm is how many consecutive accepted windows make a detection.
	Confirm int
	// Refractory is how long after a detection, measured from its window's
	// end, no window counts toward another one. A refractory period of at
	// least Window keeps one utterance from firing twice.
	Refractory int
}

// Validate checks the geometry.
func (c Config) Validate() error {
	switch {
	case c.Window < 1:
		return errors.New("window must be positive")
	case c.Hop < 1 || c.Hop > c.Window:
		return fmt.Errorf("hop must be between 1 and the window (%d samples)", c.Window)
	case c.Confirm < 1:
		return errors.New("confirm must be at least 1")
	case c.Refractory < 0:
		return errors.New("refractory period must be nonnegative")
	}
	return nil
}

// Window is one scored span of the stream. Start and End are sample offsets
// from the start of the stream, End exclusive. Samples is reused by the next
// window; copy it to keep it.
type Window struct {
	Start, End int
	Samples    []float32
}

// Event reports one scored window. Detected marks the window that completed
// a detection.
type Event[T any] struct {
	Window   Window
	Result   T
	Accepted bool
	Detected bool
}

// Score scores one window and reports whether it is accepted.
type Score[T any] func(context.Context, Window) (T, bool, error)

// Run reads s16le samples from src until EOF or cancellation, scoring a window
// every Hop samples once Window samples have arrived, and calls emit for every
// scored window. A trailing partial hop is not scored. Run returns nil at EOF
// and ctx.Err() when canceled; a blocked read is abandoned, not interrupted.
func Run[T any](ctx context.Context, src io.Reader, c Config, score Score[T], emit func(Event[T]) error) error {
	if err := c.Validate(); err != nil {
		return err
	}
	chunks := make(chan []float32, 4)
	failed := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(chunks)
		raw := make([]byte, 2*c.Hop)
		for {
			n, err := io.ReadFull(src, raw)
			if n >= 2 {
				chunk := make([]float32, n/2)
				for i := range chunk {
					chunk[i] = float32(int16(binary.LittleEndian.Uint16(raw[2*i:]))) / 32768
				}
				select {
				case chunks <- chunk:
				case <-done:
					return
				}
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			}
			if err != nil {
				failed <- err
				return
			}
		}
	}()

	buffer := make([]float32, 0, c.Window+c.Hop)
	total, pending, streak := 0, 0, 0
	last := -1 // End of the last detection
	for {
		var chunk []float32
		var ok bool
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok = <-chunks:
		}
		if !ok {
			select {
			case err := <-failed:
				return fmt.Errorf("read audio: %w", err)
			default:
				return nil
			}
		}
		buffer = append(buffer, chunk...)
		if len(buffer) > c.Window {
			buffer = buffer[:copy(buffer, buffer[len(buffer)-c.Window:])]
		}
		total += len(chunk)
		pending += len(chunk)
		if total < c.Window || pending < c.Hop {
			continue
		}
		pending = 0
		w := Window{Start: total - c.Window, End: total, Samples: buffer}
		result, accepted, err := score(ctx, w)
		if err != nil {
			return err
		}
		event := Event[T]{Window: w, Result: result, Accepted: accepted}
		switch {
		case last >= 0 && w.End-last < c.Refractory:
			streak = 0
		case accepted:
			streak++
		default:
			streak = 0
		}
		if streak >= c.Confirm {
			event.Detected = true
			last, streak = w.End, 0
		}
		if err := emit(event); err != nil {
			return err
		}
	}
}
