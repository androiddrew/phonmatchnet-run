package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	phonmatchnet "github.com/androiddrew/go-phonmatchnet"
)

func execute(t *testing.T, stdin []byte, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCommand()
	var out, errOut bytes.Buffer
	cmd.SetIn(bytes.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// Input problems are reported before ONNX Runtime is loaded, so these run
// without it: "not-a-runtime" would fail if New were reached.
func TestInputErrors(t *testing.T) {
	t.Setenv(ORTEnv, "")
	clip := "../../testdata/said-mister.wav"
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"no runtime", []string{"detect", clip, "said mister"}, ORTEnv},
		{"missing file", []string{"detect", "nope.wav", "said mister", "--ort", "not-a-runtime"}, "no such file"},
		{"not WAV", []string{"detect", "cli.go", "said mister", "--ort", "not-a-runtime"}, "ffmpeg"},
		{"digits", []string{"detect", clip, "route 66", "--ort", "not-a-runtime"}, "spell out numbers"},
		{"threads", []string{"detect", clip, "said mister", "--ort", "not-a-runtime", "--threads", "0"}, "--threads"},
		{"threshold range", []string{"detect", clip, "said mister", "--ort", "not-a-runtime", "--threshold", "1.5"}, "--threshold"},
		{"threshold and far", []string{"detect", clip, "said mister", "--ort", "not-a-runtime", "--threshold", "0.5", "--far", "0.1"}, "none of the others"},
		{"args", []string{"detect", clip}, "accepts 2 arg(s)"},
	} {
		out, _, err := execute(t, nil, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) || out != "" {
			t.Errorf("%s: %v, output %q; want error containing %q", c.name, err, out, c.want)
		}
	}
}

func TestDetect(t *testing.T) {
	library := os.Getenv("PHONMATCHNET_TEST_ORT")
	if library == "" {
		t.Skip("set PHONMATCHNET_TEST_ORT to an ONNX Runtime library for native tests")
	}
	t.Setenv(ORTEnv, library)
	clip, err := os.ReadFile("../../testdata/said-mister.wav")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name     string
		stdin    []byte
		args     []string
		accepted bool
	}{
		{"accepted", nil, []string{"detect", "../../testdata/said-mister.wav", "said mister"}, true},
		{"rejected", nil, []string{"detect", "../../testdata/said-mister.wav", "hey computer"}, false},
		{"stdin", clip, []string{"detect", "-", "Said, Mister!"}, true},
		// Confusable: rejected at FAR 0.01, accepted at 0.1 (see the README).
		{"strict", nil, []string{"detect", "../../testdata/af_heart-hey-commuter.wav", "hey computer"}, false},
		{"sensitive", nil, []string{"detect", "../../testdata/af_heart-hey-commuter.wav", "hey computer", "--far", "0.1"}, true},
		// A custom threshold decides between the two calibrated points (score 0.48).
		{"custom low", nil, []string{"detect", "../../testdata/af_heart-hey-commuter.wav", "hey computer", "--threshold", "0.4"}, true},
		{"custom high", nil, []string{"detect", "../../testdata/said-mister.wav", "said mister", "--threshold", "0.999"}, false},
	} {
		out, _, err := execute(t, c.stdin, c.args...)
		if c.accepted && err != nil || !c.accepted && !errors.Is(err, ErrRejected) {
			t.Fatalf("%s: %v", c.name, err)
		}
		var r report
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, out)
		}
		if r.Accepted != c.accepted || r.Score < 0 || r.Score > 1 || r.Seconds <= 0 || r.Audio != c.args[1] {
			t.Errorf("%s: %+v", c.name, r)
		}
		custom := strings.Contains(strings.Join(c.args, " "), "--threshold")
		if custom != (r.TargetFAR == nil) || custom && r.Threshold != mustFloat(t, c.args[len(c.args)-1]) {
			t.Errorf("%s: threshold %v, target_far %v", c.name, r.Threshold, r.TargetFAR)
		}
	}
	if _, _, err := execute(t, nil, "detect", "../../testdata/said-mister.wav", "said mister", "--far", "0.05"); !errors.Is(err, phonmatchnet.ErrUnknownFAR) {
		t.Errorf("uncalibrated FAR: %v", err)
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestListenInputErrors(t *testing.T) {
	t.Setenv(ORTEnv, "")
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"no runtime", []string{"listen", "said mister"}, ORTEnv},
		{"window", []string{"listen", "said mister", "--ort", "not-a-runtime", "--window", "3s"}, "--window"},
		{"hop", []string{"listen", "said mister", "--ort", "not-a-runtime", "--hop", "5ms"}, "--hop"},
		{"hop over window", []string{"listen", "said mister", "--ort", "not-a-runtime", "--hop", "2s", "--window", "1s"}, "--hop"},
		{"confirm", []string{"listen", "said mister", "--ort", "not-a-runtime", "--confirm", "0"}, "confirm"},
		{"refractory", []string{"listen", "said mister", "--ort", "not-a-runtime", "--refractory", "-1s"}, "refractory"},
		{"digits", []string{"listen", "route 66", "--ort", "not-a-runtime"}, "spell out numbers"},
		{"missing input", []string{"listen", "said mister", "--ort", "not-a-runtime", "--input", "nope.wav"}, "no such file"},
	} {
		out, _, err := execute(t, nil, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) || out != "" {
			t.Errorf("%s: %v, output %q; want error containing %q", c.name, err, out, c.want)
		}
	}
}

// pcmOf concatenates WAV clips and silences (in samples) into one raw s16le stream.
func pcmOf(t *testing.T, parts ...any) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, part := range parts {
		switch p := part.(type) {
		case int:
			out.Write(make([]byte, 2*p))
		case string:
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			out.Write(data[44:]) // the fixtures have plain 44-byte headers
		}
	}
	return out.Bytes()
}

func TestListen(t *testing.T) {
	library := os.Getenv("PHONMATCHNET_TEST_ORT")
	if library == "" {
		t.Skip("set PHONMATCHNET_TEST_ORT to an ONNX Runtime library for native tests")
	}
	t.Setenv(ORTEnv, library)
	const second = 16000
	said, commuter := "../../testdata/said-mister.wav", "../../testdata/af_heart-hey-commuter.wav"
	// "said mister" twice, with silence and a different phrase between.
	stream := pcmOf(t, second, said, second, commuter, second, said, second)
	saidSeconds := float64(len(pcmOf(t, said))/2) / second
	starts := []float64{1, 1 + saidSeconds + 1 + float64(len(pcmOf(t, commuter))/2)/second + 1}
	type line struct {
		Start, End float64
		Detected   bool
		Score      float64
	}
	parse := func(out string) (lines []line) {
		for _, text := range strings.Split(strings.TrimSpace(out), "\n") {
			var e event
			if err := json.Unmarshal([]byte(text), &e); err != nil {
				t.Fatalf("%v: %q", err, text)
			}
			lines = append(lines, line{e.StartSeconds, e.EndSeconds, e.Detected, e.Score})
		}
		return lines
	}

	out, errOut, err := execute(t, stream, "listen", "said mister")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, `listening for "said mister"`) {
		t.Errorf("status: %q", errOut)
	}
	detections := parse(out)
	if len(detections) != 2 {
		t.Fatalf("detections %+v, want one per utterance", detections)
	}
	// The model may fire before the last few milliseconds of an utterance
	// arrive, so each detection must contain most of its utterance.
	for i, start := range starts {
		end := start + saidSeconds
		if d := detections[i]; !d.Detected || min(d.End, end)-max(d.Start, start) < 0.8*saidSeconds {
			t.Errorf("detection %d %+v does not cover the utterance at %.2f-%.2f s", i, d, start, end)
		}
	}

	// --all prints every window: one per 250 ms hop once 1.5 s have arrived.
	out, _, err = execute(t, stream, "listen", "said mister", "--all", "--confirm", "2")
	if err != nil {
		t.Fatal(err)
	}
	windows, detected := parse(out), 0
	total := float64(len(stream)/2) / second
	if want := int((total-1.5)/0.25) + 1; len(windows) != want {
		t.Errorf("%d windows, want %d", len(windows), want)
	}
	for _, w := range windows {
		if w.Detected {
			detected++
		}
	}
	if detected != 2 {
		t.Errorf("--confirm 2: %d detections", detected)
	}

	// A WAV stream from a file gives the same result as raw stdin.
	wavStream := append([]byte(nil), mustRead(t, said)[:44]...)
	wavStream = append(wavStream, stream...)
	path := t.TempDir() + "/stream.wav"
	if err := os.WriteFile(path, wavStream, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _, err = execute(t, nil, "listen", "said mister", "--input", path); err != nil || len(parse(out)) != 2 {
		t.Errorf("WAV input: %v %q", err, out)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
