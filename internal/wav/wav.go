// Package wav reads mono 16-bit PCM audio: whole WAV clips for detect, and
// raw or WAV streams for listen.
package wav

import (
	"bufio"
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

// Stream returns the 16-bit little-endian PCM in r for continuous input. A
// WAV stream (starting with RIFF) must be mono 16-bit PCM at rate; its header
// is consumed and the data chunk's size is ignored, since capture tools write
// a placeholder. Anything else is passed through as raw s16le. The header is
// read on the first Read, so Stream never blocks and format errors surface
// as read errors.
func Stream(r io.Reader, rate int) io.Reader { return &stream{source: r, rate: rate} }

type stream struct {
	source io.Reader
	rate   int
	pcm    io.Reader
	err    error
}

func (s *stream) Read(p []byte) (int, error) {
	if s.pcm == nil && s.err == nil {
		s.pcm, s.err = openStream(s.source, s.rate)
	}
	if s.err != nil {
		return 0, s.err
	}
	return s.pcm.Read(p)
}

func openStream(r io.Reader, rate int) (io.Reader, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(4)
	if err != nil && len(magic) == 0 {
		if err == io.EOF {
			return br, nil
		}
		return nil, err
	}
	if string(magic) != "RIFF" {
		return br, nil
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(br, header); err != nil || string(header[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a WAV stream: %w", ErrFormat)
	}
	seenFormat := false
	for {
		chunk := make([]byte, 8)
		if _, err := io.ReadFull(br, chunk); err != nil {
			return nil, fmt.Errorf("no data chunk: %w", ErrFormat)
		}
		id, size := string(chunk[:4]), int64(binary.LittleEndian.Uint32(chunk[4:]))
		if id == "data" {
			if !seenFormat {
				return nil, fmt.Errorf("data before fmt chunk: %w", ErrFormat)
			}
			return br, nil
		}
		if id != "fmt " {
			// Skip metadata, word-aligned.
			if _, err := io.CopyN(io.Discard, br, size+size%2); err != nil {
				return nil, fmt.Errorf("truncated %q chunk: %w", id, ErrFormat)
			}
			continue
		}
		if size < 16 || size > 1024 {
			return nil, fmt.Errorf("bad fmt chunk: %w", ErrFormat)
		}
		body := make([]byte, size+size%2)
		if _, err := io.ReadFull(br, body); err != nil {
			return nil, fmt.Errorf("truncated fmt chunk: %w", ErrFormat)
		}
		tag := binary.LittleEndian.Uint16(body[0:])
		if tag == formatExtensible && size >= 26 {
			tag = binary.LittleEndian.Uint16(body[24:])
		}
		if tag != formatPCM || binary.LittleEndian.Uint16(body[2:]) != 1 || binary.LittleEndian.Uint16(body[14:]) != 16 {
			return nil, ErrFormat
		}
		if got := int(binary.LittleEndian.Uint32(body[4:])); got != rate {
			return nil, fmt.Errorf("WAV stream is %d Hz, expected %d: %w", got, rate, ErrFormat)
		}
		seenFormat = true
	}
}
