package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	phonmatchnet "github.com/androiddrew/go-phonmatchnet"
	"github.com/spf13/cobra"

	"github.com/androiddrew/phonmatchnet-run/internal/listen"
	"github.com/androiddrew/phonmatchnet-run/internal/wav"
)

// event is one listen output line.
type event struct {
	Time         string  `json:"time"` // wall clock when the window was scored
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Detected     bool    `json:"detected"`
	decision
}

func listenCommand() *cobra.Command {
	var model modelOptions
	var input string
	var window, hop, refractory time.Duration
	var confirm int
	var all bool
	cmd := &cobra.Command{
		Use:   "listen KEYWORD",
		Short: "Watch a live 16 kHz mono audio stream for a keyword",
		Long: `Watch a continuous 16 kHz mono 16-bit audio stream for a typed keyword. The
stream is read from standard input (or --input) as raw s16le or as WAV, so any
capture tool can feed it. A window of recent audio is scored every hop, and
one JSON line is printed per detection.

Debouncing: a detection needs --confirm consecutive accepted windows, and for
--refractory after it (default: the window length) no window counts toward
another, so one utterance fires once.

Windows are scored on far more audio than single clips, so the calibrated
false-accept rates do not hold; expect to raise --threshold or --confirm for
your microphone and room. --all prints every window's score for tuning.

Stops cleanly at end of input or on Ctrl-C (exit status 0).
` + backendHelp,
		Example: `  # Linux (ALSA)
  arecord -q -f S16_LE -r 16000 -c 1 | phonmatchnet-run listen "hey computer"
  # Linux (PipeWire)
  pw-record --rate 16000 --channels 1 --format s16 - | phonmatchnet-run listen "hey computer"
  # macOS
  ffmpeg -loglevel error -f avfoundation -i ":0" -ac 1 -ar 16000 -f s16le - | phonmatchnet-run listen "hey computer"
  # A long recording, printing every window's score
  phonmatchnet-run listen "hey computer" --input recording.wav --all`,
		Args: cobra.ExactArgs(1),
	}
	model.register(cmd)
	f := cmd.Flags()
	f.StringVar(&input, "input", "-", "Audio stream: - for standard input, or a raw s16le or WAV file")
	f.DurationVar(&window, "window", 1500*time.Millisecond, "Audio scored per window, 0.5s to 2s")
	f.DurationVar(&hop, "hop", 250*time.Millisecond, "Time between scored windows, 10ms to the window length")
	f.IntVar(&confirm, "confirm", 1, "Consecutive accepted windows needed for a detection")
	f.DurationVar(&refractory, "refractory", 0, "Time after a detection before the next can start (default: the window length)")
	f.BoolVar(&all, "all", false, "Print every scored window, not only detections")
	cmd.RunE = func(cmd *cobra.Command, args []string) (err error) {
		text := args[0]
		if err := model.validate(cmd); err != nil {
			return err
		}
		if !cmd.Flags().Changed("refractory") {
			refractory = window
		}
		config := listen.Config{
			Window: samples(window), Hop: samples(hop), Confirm: confirm, Refractory: samples(refractory),
		}
		if config.Window < phonmatchnet.MinSamples || config.Window > phonmatchnet.MaxSamples {
			return errors.New("--window must be between 0.5s and 2s")
		}
		if hop < 10*time.Millisecond || hop > window {
			return errors.New("--hop must be between 10ms and the window length")
		}
		if err := config.Validate(); err != nil {
			return err
		}
		if _, err := phonmatchnet.NormalizeKeyword(text); err != nil {
			return err
		}
		reader := cmd.InOrStdin()
		if input != "-" {
			f, err := os.Open(input)
			if err != nil {
				return err
			}
			defer f.Close()
			reader = f
		}
		// An interrupt is a normal way to stop listening, at any stage.
		defer func() {
			if errors.Is(err, context.Canceled) && cmd.Context().Err() != nil {
				err = nil
			}
		}()
		engine, err := model.open()
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, engine.Close()) }()
		keyword, err := engine.Keyword(cmd.Context(), text)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "listening for %q: %v windows every %v; Ctrl-C to stop\n", keyword.Text(), window, hop)
		score := func(ctx context.Context, w listen.Window) (decision, bool, error) {
			result, err := engine.Detect(ctx, phonmatchnet.Request{
				Samples: w.Samples, SampleRate: phonmatchnet.SampleRate, Keyword: keyword, FAR: model.far,
			})
			if err != nil {
				return decision{}, false, err
			}
			d := model.decide(result)
			return d, d.Accepted, nil
		}
		emit := func(e listen.Event[decision]) error {
			if !e.Detected && !all {
				return nil
			}
			return encodeLine(cmd.OutOrStdout(), event{
				Time:         time.Now().Format(time.RFC3339Nano),
				StartSeconds: seconds(e.Window.Start),
				EndSeconds:   seconds(e.Window.End),
				Detected:     e.Detected,
				decision:     e.Result,
			})
		}
		return listen.Run(cmd.Context(), wav.Stream(reader, phonmatchnet.SampleRate), config, score, emit)
	}
	return cmd
}

func samples(d time.Duration) int { return int(d * phonmatchnet.SampleRate / time.Second) }

func seconds(samples int) float64 { return float64(samples) / phonmatchnet.SampleRate }

// encodeLine writes one compact JSON line, flushed by the write itself.
func encodeLine(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}
