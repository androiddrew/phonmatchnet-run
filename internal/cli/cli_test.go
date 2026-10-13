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
