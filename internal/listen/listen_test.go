package listen

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"testing"
)

// pcm encodes samples 0, 1, 2, ... n-1 as s16le, so a window's first sample
// value is its start offset (scaled by 1/32768).
func pcm(n int) []byte {
	var b bytes.Buffer
	for i := range n {
		_ = binary.Write(&b, binary.LittleEndian, int16(i))
	}
	return b.Bytes()
}

// run scores windows as accepted when their End is in accept, returning the
// emitted events' End offsets and detections.
func run(t *testing.T, src io.Reader, c Config, accept ...int) (ends, detected []int) {
	t.Helper()
	set := map[int]bool{}
	for _, end := range accept {
		set[end] = true
	}
	err := Run(context.Background(), src, c, func(_ context.Context, w Window) (int, bool, error) {
		if len(w.Samples) != c.Window || w.Samples[0] != float32(w.Start)/32768 || w.Samples[c.Window-1] != float32(w.End-1)/32768 {
			t.Fatalf("window [%d,%d) has wrong samples", w.Start, w.End)
		}
		return w.End, set[w.End], nil
	}, func(e Event[int]) error {
		if e.Result != e.Window.End {
			t.Fatalf("result %d for window ending %d", e.Result, e.Window.End)
		}
		ends = append(ends, e.Window.End)
		if e.Detected {
			detected = append(detected, e.Window.End)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ends, detected
}

func TestWindows(t *testing.T) {
	// Window 4, hop 2: windows end every 2 samples once 4 have arrived; the
	// trailing partial hop (sample 10) and the odd byte are not scored.
	ends, _ := run(t, bytes.NewReader(append(pcm(11), 7)), Config{Window: 4, Hop: 2, Confirm: 1})
	if want := []int{4, 6, 8, 10}; !reflect.DeepEqual(ends, want) {
		t.Errorf("window ends %v, want %v", ends, want)
	}
	// A window that is not a multiple of the hop starts at the next hop.
	ends, _ = run(t, bytes.NewReader(pcm(12)), Config{Window: 5, Hop: 3, Confirm: 1})
	if want := []int{6, 9, 12}; !reflect.DeepEqual(ends, want) {
		t.Errorf("window ends %v, want %v", ends, want)
	}
	ends, _ = run(t, bytes.NewReader(pcm(3)), Config{Window: 4, Hop: 2, Confirm: 1})
	if len(ends) != 0 {
		t.Errorf("short stream scored %v", ends)
	}
}

func TestDebounce(t *testing.T) {
	stream := func() io.Reader { return bytes.NewReader(pcm(40)) }
	for _, c := range []struct {
		name   string
		config Config
		accept []int
		want   []int
	}{
		{"every accept", Config{Window: 4, Hop: 2, Confirm: 1}, []int{6, 8, 20}, []int{6, 8, 20}},
		{"refractory", Config{Window: 4, Hop: 2, Confirm: 1, Refractory: 4}, []int{6, 8, 10, 12, 20}, []int{6, 10, 20}},
		{"confirm", Config{Window: 4, Hop: 2, Confirm: 2}, []int{6, 10, 12, 14, 16, 30}, []int{12, 16}},
		// Accepts inside the refractory period do not count toward the streak.
		{"confirm after refractory", Config{Window: 4, Hop: 2, Confirm: 2, Refractory: 6}, []int{6, 8, 10, 12, 14, 16, 18}, []int{8, 16}},
	} {
		if _, got := run(t, stream(), c.config, c.accept...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: detections at %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStops(t *testing.T) {
	score := func(context.Context, Window) (int, bool, error) { return 0, false, nil }
	emit := func(Event[int]) error { return nil }
	c := Config{Window: 4, Hop: 2, Confirm: 1}

	// Cancellation while the reader is blocked.
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, reader, c, score, emit); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
	broken := io.MultiReader(bytes.NewReader(pcm(6)), iotestErr{})
	if err := Run(context.Background(), broken, c, score, emit); err == nil || !errors.Is(err, errBroken) {
		t.Errorf("read error: %v", err)
	}
	stop := errors.New("stop")
	if err := Run(context.Background(), bytes.NewReader(pcm(10)), c, score, func(Event[int]) error { return stop }); !errors.Is(err, stop) {
		t.Errorf("emit error: %v", err)
	}
	if err := Run(context.Background(), bytes.NewReader(pcm(10)), c, func(context.Context, Window) (int, bool, error) { return 0, false, stop }, emit); !errors.Is(err, stop) {
		t.Errorf("score error: %v", err)
	}
	for _, bad := range []Config{{Window: 0, Hop: 1, Confirm: 1}, {Window: 4, Hop: 5, Confirm: 1}, {Window: 4, Hop: 2}, {Window: 4, Hop: 2, Confirm: 1, Refractory: -1}} {
		if err := Run(context.Background(), bytes.NewReader(nil), bad, score, emit); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

var errBroken = errors.New("device unplugged")

type iotestErr struct{}

func (iotestErr) Read([]byte) (int, error) { return 0, errBroken }
