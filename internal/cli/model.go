package cli

import (
	"errors"
	"fmt"
	"os"

	phonmatchnet "github.com/androiddrew/go-phonmatchnet"
	"github.com/spf13/cobra"
)

// modelOptions holds the flags detect and listen share: runtime, model and
// decision threshold.
type modelOptions struct {
	library, modelDir string
	far, threshold    float64
	threads           int
	custom            bool // --threshold was given
}

func (o *modelOptions) register(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&o.library, "ort", "", "ONNX Runtime library path or OS-loader name (or "+ORTEnv+")")
	f.Float64Var(&o.far, "far", phonmatchnet.DefaultFAR, "Calibrated operating point: target false-accept rate, 0.01 or 0.1")
	f.Float64Var(&o.threshold, "threshold", 0, "Custom score threshold in [0, 1] instead of a calibrated operating point")
	cmd.MarkFlagsMutuallyExclusive("far", "threshold")
	f.IntVar(&o.threads, "threads", 1, "ONNX Runtime intra-op threads per session")
	f.StringVar(&o.modelDir, "model-dir", "", "Override the built-in models with a directory holding phonmatchnet/, google/ and g2p/")
}

// validate resolves the runtime selector and checks the flags without loading
// ONNX Runtime.
func (o *modelOptions) validate(cmd *cobra.Command) error {
	if o.library == "" {
		o.library = os.Getenv(ORTEnv)
	}
	if o.library == "" {
		return fmt.Errorf("provide --ort with a library path or loader name (e.g. libonnxruntime.so), or set %s", ORTEnv)
	}
	if o.threads < 1 {
		return errors.New("--threads must be at least 1")
	}
	o.custom = cmd.Flags().Changed("threshold")
	if o.custom && !(o.threshold >= 0 && o.threshold <= 1) {
		return errors.New("--threshold must be within [0, 1]")
	}
	return nil
}

func (o *modelOptions) open() (*phonmatchnet.Engine, error) {
	return phonmatchnet.New(phonmatchnet.Config{ORTLibrary: o.library, ModelDir: o.modelDir, Threads: o.threads})
}

// decision is a result judged against the chosen threshold. TargetFAR shadows
// the embedded field so a custom --threshold reports null instead of a
// calibrated operating point.
type decision struct {
	phonmatchnet.Result
	TargetFAR *float64 `json:"target_far"`
}

func (o *modelOptions) decide(result phonmatchnet.Result) decision {
	d := decision{Result: result}
	if o.custom {
		d.Threshold = o.threshold
		d.Accepted = result.Score >= o.threshold
	} else {
		d.TargetFAR = &d.Result.TargetFAR
	}
	return d
}
