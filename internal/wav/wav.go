// Package wav reads the clips phonmatchnet-run scores: mono 16-bit PCM WAV.
package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxBytes bounds how much of a file Decode reads. A 2-second clip needs
// 64,000 bytes of samples; the rest allows for metadata chunks.
const MaxBytes = 1 << 20

// ErrFormat reports a file that is not mono 16-bit PCM WAV.
var ErrFormat = errors.New("expected a mono, 16-bit PCM WAV file; convert it first, e.g. ffmpeg -i in.wav -ac 1 -ar 16000 -sample_fmt s16 out.wav")

const (
	formatPCM        = 1
	formatExtensible = 0xfffe
)

// Decode reads a mono 16-bit PCM WAV and returns its sample rate and samples
// scaled into [-1, 1) as naptrap.load_wav does (sample / 32768). It does not
// resample.
func Decode(r io.Reader) (rate int, samples []float32, err error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return 0, nil, err
	}
	if len(data) > MaxBytes {
		return 0, nil, fmt.Errorf("WAV file exceeds %d bytes; clips must be 0.5-2 seconds", MaxBytes)
	}
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, nil, fmt.Errorf("not a WAV file: %w", ErrFormat)
	}
	var tag, channels, bits uint16
	seenFormat := false
	for chunk := data[12:]; len(chunk) >= 8; {
		id, size := string(chunk[:4]), uint64(binary.LittleEndian.Uint32(chunk[4:8]))
		if size > uint64(len(chunk)-8) {
			return 0, nil, fmt.Errorf("truncated %q chunk: %w", id, ErrFormat)
		}
		body := chunk[8 : 8+size]
		switch id {
		case "fmt ":
			if len(body) < 16 {
				return 0, nil, fmt.Errorf("short fmt chunk: %w", ErrFormat)
			}
			tag = binary.LittleEndian.Uint16(body[0:])
			channels = binary.LittleEndian.Uint16(body[2:])
			rate = int(binary.LittleEndian.Uint32(body[4:]))
			bits = binary.LittleEndian.Uint16(body[14:])
			// WAVE_FORMAT_EXTENSIBLE carries the real format in its sub-format GUID.
			if tag == formatExtensible && len(body) >= 26 {
				tag = binary.LittleEndian.Uint16(body[24:])
			}
			seenFormat = true
		case "data":
			if !seenFormat || tag != formatPCM || channels != 1 || bits != 16 {
				return 0, nil, ErrFormat
			}
			samples = make([]float32, size/2)
			for i := range samples {
				samples[i] = float32(int16(binary.LittleEndian.Uint16(body[2*i:]))) / 32768
			}
			return rate, samples, nil
		}
		chunk = chunk[8+size:]
		if size%2 == 1 && len(chunk) > 0 {
			chunk = chunk[1:] // chunks are word-aligned
		}
	}
	return 0, nil, fmt.Errorf("no data chunk: %w", ErrFormat)
}
