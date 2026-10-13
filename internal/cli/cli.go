// Package cli implements phonmatchnet-run, a command-line keyword detector.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	phonmatchnet "github.com/androiddrew/go-phonmatchnet"
	"github.com/spf13/cobra"

	"github.com/androiddrew/phonmatchnet-run/internal/wav"
)

// ORTEnv names the environment variable that selects ONNX Runtime when --ort
// is not given.
const ORTEnv = "PHONMATCHNET_RUN_ORT_LIBRARY"

// ErrRejected is returned by detect after printing a result whose keyword was
// not accepted, so the process can exit 1.
var ErrRejected = errors.New("keyword rejected")

const backendHelp = `
Requires an installed ONNX Runtime (tested with 1.22). Select it with --ort or
` + ORTEnv + `: a library path or an OS-loader name such as libonnxruntime.so.
The models are built in.`

// NewCommand returns the root command.
func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use: "phonmatchnet-run", Short: "Text-defined keyword detection for short clips",
		Long:         "Text-defined keyword detection for short clips with PhonMatchNet.\n" + backendHelp,
		Version:      buildVersion(),
		SilenceUsage: true, SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(detectCommand())
	return root
}

// report is detect's JSON output.
type report struct {
	Audio   string  `json:"audio"`
	Seconds float64 `json:"seconds"`
	phonmatchnet.Result
}

func detectCommand() *cobra.Command {
	var library, modelDir string
	var far float64
	var threads int
	cmd := &cobra.Command{
		Use:   "detect AUDIO KEYWORD",
		Short: "Score one 0.5-2 s mono 16 kHz 16-bit WAV clip against a keyword",
		Long: `Score one 0.5-2 s mono 16 kHz 16-bit WAV clip against a typed keyword and
print the result as JSON. AUDIO may be - to read standard input.

Exit status: 0 if the keyword was accepted, 1 if it was rejected, 2 on error.
The calibrated thresholds do not transfer reliably to other audio; for a real
deployment, choose your own threshold on the printed score.
` + backendHelp,
		Example: `  phonmatchnet-run detect clip.wav "hey computer"
  phonmatchnet-run detect clip.wav "hey computer" --far 0.1
  ffmpeg -i in.m4a -ac 1 -ar 16000 -sample_fmt s16 -f wav - | phonmatchnet-run detect - "okay naptrap"`,
		Args: cobra.ExactArgs(2),
	}
	f := cmd.Flags()
	f.StringVar(&library, "ort", "", "ONNX Runtime library path or OS-loader name (or "+ORTEnv+")")
	f.Float64Var(&far, "far", phonmatchnet.DefaultFAR, "Calibrated operating point: target false-accept rate, 0.01 or 0.1")
	f.IntVar(&threads, "threads", 1, "ONNX Runtime intra-op threads per session")
	f.StringVar(&modelDir, "model-dir", "", "Override the built-in models with a directory holding phonmatchnet/, google/ and g2p/")
	cmd.RunE = func(cmd *cobra.Command, args []string) (err error) {
		path, text := args[0], args[1]
		if library == "" {
			library = os.Getenv(ORTEnv)
		}
		if library == "" {
			return fmt.Errorf("provide --ort with a library path or loader name (e.g. libonnxruntime.so), or set %s", ORTEnv)
		}
		if threads < 1 {
			return errors.New("--threads must be at least 1")
		}
		rate, samples, err := readClip(cmd, path)
		if err != nil {
			return err
		}
		// Check the clip and keyword before loading ONNX Runtime.
		if _, err = phonmatchnet.NormalizeKeyword(text); err != nil {
			return err
		}
		if rate != phonmatchnet.SampleRate {
			return fmt.Errorf("%s is %d Hz; convert it first, e.g. ffmpeg -i in.wav -ac 1 -ar 16000 -sample_fmt s16 out.wav", path, rate)
		}
		engine, err := phonmatchnet.New(phonmatchnet.Config{ORTLibrary: library, ModelDir: modelDir, Threads: threads})
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, engine.Close()) }()
		keyword, err := engine.Keyword(cmd.Context(), text)
		if err != nil {
			return err
		}
		result, err := engine.Detect(cmd.Context(), phonmatchnet.Request{
			Samples: samples, SampleRate: rate, Keyword: keyword, FAR: far,
		})
		if err != nil {
			return err
		}
		out := report{Audio: path, Seconds: float64(len(samples)) / float64(rate), Result: result}
		if err := encode(cmd.OutOrStdout(), out); err != nil {
			return err
		}
		if !result.Accepted {
			return ErrRejected
		}
		return nil
	}
	return cmd
}

func readClip(cmd *cobra.Command, path string) (int, []float32, error) {
	reader := cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return 0, nil, err
		}
		defer f.Close()
		reader = f
	}
	rate, samples, err := wav.Decode(reader)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", path, err)
	}
	return rate, samples, nil
}

func encode(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

// version is set at release time with -ldflags "-X github.com/androiddrew/phonmatchnet-run/internal/cli.version=v0.1.0".
var version string

// buildVersion prefers the linker-set version, then the module version recorded
// by "go install module@version", then "devel".
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}
