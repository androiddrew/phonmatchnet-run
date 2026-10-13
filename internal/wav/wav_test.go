package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// build writes a WAV with the given format fields, an optional odd-sized
// metadata chunk before the data, and the samples.
func build(tag, channels uint16, rate uint32, bits uint16, extensible bool, samples []int16) []byte {
	var fmtChunk bytes.Buffer
	fields := []any{tag, channels, rate, rate * uint32(channels) * uint32(bits) / 8, channels * bits / 8, bits}
	if extensible {
		fields[0] = uint16(formatExtensible)
		fields = append(fields, uint16(22), bits, uint32(4), tag, [14]byte{})
	}
	for _, field := range fields {
		_ = binary.Write(&fmtChunk, binary.LittleEndian, field)
	}
	var body bytes.Buffer
	body.WriteString("WAVE")
	chunk := func(id string, data []byte) {
		body.WriteString(id)
		_ = binary.Write(&body, binary.LittleEndian, uint32(len(data)))
		body.Write(data)
		if len(data)%2 == 1 {
			body.WriteByte(0)
		}
	}
	chunk("fmt ", fmtChunk.Bytes())
	chunk("LIST", []byte("odd"))
	var data bytes.Buffer
	_ = binary.Write(&data, binary.LittleEndian, samples)
	chunk("data", data.Bytes())
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

func TestDecode(t *testing.T) {
	samples := []int16{0, 16384, -32768, 32767}
	want := []float32{0, 0.5, -1, 32767.0 / 32768}
	for _, extensible := range []bool{false, true} {
		rate, got, err := Decode(bytes.NewReader(build(formatPCM, 1, 16000, 16, extensible, samples)))
		if err != nil || rate != 16000 || len(got) != len(want) {
			t.Fatalf("extensible=%v: %d %v %v", extensible, rate, got, err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("extensible=%v sample %d: %v, want %v", extensible, i, got[i], want[i])
			}
		}
	}
	// The sample rate is reported, not checked; the engine rejects it.
	if rate, _, err := Decode(bytes.NewReader(build(formatPCM, 1, 44100, 16, false, samples))); err != nil || rate != 44100 {
		t.Errorf("44.1 kHz: %d %v", rate, err)
	}
}

func TestDecodeRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"stereo":    build(formatPCM, 2, 16000, 16, false, []int16{1, 2}),
		"8-bit":     build(formatPCM, 1, 16000, 8, false, []int16{1}),
		"float":     build(3, 1, 16000, 32, false, []int16{1, 2}),
		"not WAV":   []byte("ID3 an mp3 file"),
		"truncated": build(formatPCM, 1, 16000, 16, false, []int16{1, 2})[:50],
	} {
		if _, _, err := Decode(bytes.NewReader(data)); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := Decode(bytes.NewReader(make([]byte, MaxBytes+1))); err == nil {
		t.Error("oversized input accepted")
	}
}

func TestStream(t *testing.T) {
	samples := []int16{1, 2, 3}
	pcm := []byte{1, 0, 2, 0, 3, 0}
	for name, input := range map[string][]byte{
		"raw":        pcm,
		"wav":        build(formatPCM, 1, 16000, 16, false, samples),
		"extensible": build(formatPCM, 1, 16000, 16, true, samples),
	} {
		got, err := io.ReadAll(Stream(bytes.NewReader(input), 16000))
		if err != nil || !bytes.Equal(got, pcm) {
			t.Errorf("%s: %v %v", name, got, err)
		}
	}
	// Capture tools write a placeholder data size; the stream runs to EOF.
	streaming := build(formatPCM, 1, 16000, 16, false, samples)
	binary.LittleEndian.PutUint32(streaming[len(streaming)-len(pcm)-4:], 0xffffffff)
	if got, err := io.ReadAll(Stream(bytes.NewReader(append(streaming, 4, 0)), 16000)); err != nil || !bytes.Equal(got, append(pcm, 4, 0)) {
		t.Errorf("placeholder size: %v %v", got, err)
	}
	if got, err := io.ReadAll(Stream(bytes.NewReader(nil), 16000)); err != nil || len(got) != 0 {
		t.Errorf("empty: %v %v", got, err)
	}
	for name, input := range map[string][]byte{
		"rate":   build(formatPCM, 1, 44100, 16, false, samples),
		"stereo": build(formatPCM, 2, 16000, 16, false, samples),
		"header": []byte("RIFF\x00\x00\x00\x00WAV"),
	} {
		if _, err := io.ReadAll(Stream(bytes.NewReader(input), 16000)); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
