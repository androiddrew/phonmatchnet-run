# phonmatchnet-run

A command-line **text-defined keyword detector**. Give it a short audio clip
and a keyword typed as text, and it prints the probability that the keyword was
spoken; or pipe it a microphone stream and it prints each time the keyword is
said. New wake words need no recordings or training.

It runs naptrap's independently trained English
[PhonMatchNet](https://huggingface.co/androiddrew/naptrap-phonmatchnet-en)
through [go-phonmatchnet](https://github.com/androiddrew/go-phonmatchnet). The
models are built into the binary; the only external requirement is an
installed ONNX Runtime. `detect` follows `naptrap detect`.

## Install

```bash
go install github.com/androiddrew/phonmatchnet-run/cmd/phonmatchnet-run@latest
```

Requires Go 1.25 or newer with CGO, and ONNX Runtime 1.22 (tested with 1.22.0
and 1.22.2 on Linux/amd64), e.g. from the
[ONNX Runtime releases](https://github.com/microsoft/onnxruntime/releases).

## Usage

```bash
export PHONMATCHNET_RUN_ORT_LIBRARY=/path/to/libonnxruntime.so   # or pass --ort
phonmatchnet-run detect said-mister.wav "said mister"
```

```json
{
  "audio": "said-mister.wav",
  "seconds": 0.54,
  "keyword": "said mister",
  "score": 0.9978187183303515,
  "logit": 6.125658988952637,
  "threshold": 0.7149204015731812,
  "accepted": true,
  "timings": {
    "embedding_seconds": 0.002619669,
    "detector_seconds": 0.000784665
  },
  "target_far": 0.01
}
```

The exit status is **0 if the keyword was accepted, 1 if it was rejected and 2
on error**, so scripts can branch on it directly. `score` is the keyword-match
probability (0–1); `keyword` is the normalized text that was scored.

| Flag | Meaning |
|---|---|
| `--ort` | ONNX Runtime library path or OS-loader name; default `$PHONMATCHNET_RUN_ORT_LIBRARY` |
| `--far` | Calibrated operating point, `0.01` (default, strict) or `0.1` (sensitive) |
| `--threshold` | Your own score threshold in [0, 1] instead of `--far`; `target_far` is then `null` |
| `--threads` | ONNX Runtime intra-op threads per session (default 1) |
| `--model-dir` | Use model files from a directory instead of the built-in ones (see go-phonmatchnet's ASSETS.md) |

### Audio

`detect` scores **one mono 16 kHz 16-bit PCM WAV clip of 0.5–2 seconds**. It
does not resample, trim or rescale. Convert other files with ffmpeg, or pipe
them in with `-` as the file name:

```bash
ffmpeg -i input.m4a -ac 1 -ar 16000 -sample_fmt s16 clip.wav
ffmpeg -i input.m4a -ac 1 -ar 16000 -sample_fmt s16 -f wav - | phonmatchnet-run detect - "okay naptrap"
```

### Keywords

Keywords are English words; accents and punctuation are removed and case is
ignored, so `"Hey, Computer!"` scores as `hey computer`. Spell out numbers
("route sixty six").

### Thresholds

The two operating points were fitted on held-out LibriSpeech speakers for a 1%
or 10% per-clip false-accept rate. **These targets do not hold on other
audio.** Similar-sounding phrases are often confused: a synthesized
"hey commuter" scores 0.48 for the keyword "hey computer", rejected at `0.01`
but accepted at `0.1`. For a real product, record positive and negative clips
in your setting, choose your own threshold on `score`, and pass it with
`--threshold`:

```bash
phonmatchnet-run detect clip.wav "hey computer" --threshold 0.9
```

`accepted` and the exit status then follow `score >= threshold`, and
`target_far` is `null` because no calibrated operating point was used.

## Listening to a microphone

`listen` watches a continuous stream for a keyword and prints one JSON line per
detection. It reads 16 kHz mono 16-bit audio from standard input, as raw s16le
or WAV, so any capture tool can feed it:

```bash
# Linux (ALSA)
arecord -q -f S16_LE -r 16000 -c 1 | phonmatchnet-run listen "hey computer"
# Linux (PipeWire)
pw-record --rate 16000 --channels 1 --format s16 - | phonmatchnet-run listen "hey computer"
# macOS (list devices with: ffmpeg -f avfoundation -list_devices true -i "")
ffmpeg -loglevel error -f avfoundation -i ":0" -ac 1 -ar 16000 -f s16le - | phonmatchnet-run listen "hey computer"
# Windows
ffmpeg -loglevel error -f dshow -i audio="Microphone" -ac 1 -ar 16000 -f s16le - | phonmatchnet-run listen "hey computer"
```

```json
{"time":"2026-10-13T11:40:02.51-04:00","start_seconds":19,"end_seconds":20.5,"detected":true,"keyword":"hey computer","score":0.994,"logit":5.12,"threshold":0.7149,"accepted":true,"timings":{…},"target_far":0.01}
```

`start_seconds` and `end_seconds` locate the detecting window in the stream;
`time` is the wall clock when it was scored. A 1.5 s window is scored every
250 ms, which takes about 2% of one CPU core. `listen` stops at the end of the
input or on Ctrl-C, with exit status 0. `--input FILE` scans a recording
instead of standard input. Input without a WAV header is read as raw s16le.

| Flag | Default | Meaning |
|---|---|---|
| `--window` | `1.5s` | Audio scored per window, 0.5–2 s |
| `--hop` | `250ms` | Time between scored windows |
| `--confirm` | `1` | Consecutive accepted windows needed for a detection |
| `--refractory` | window length | After a detection, time before windows count toward the next one, so one utterance fires once |
| `--all` | off | Print every scored window, with `detected` marking detections, for tuning |
| `--input` | `-` | Read a file instead of standard input |

`--far`, `--threshold`, `--threads`, `--model-dir` and `--ort` work as for
`detect`.

**Tuning.** A listener scores four windows a second, so false accepts add up
much faster than with single clips; the calibrated false-accept rates do not
describe false alarms per hour, which have not been measured. Run with
`--all` while you speak the keyword and while you talk normally, then pick a
`--threshold` between the two score ranges. `--confirm 2` ignores single-window
spikes at the cost of 250 ms latency. Similar-sounding phrases still fire: in
our tests a synthesized "hey commuter" triggered "hey computer" with a score of
0.994, which no threshold separates.

### Reacting to detections

Each detection is written as one line the moment it happens, so a shell loop
can act on it. To play a sound when the wake word is heard:

```bash
arecord -q -f S16_LE -r 16000 -c 1 \
  | phonmatchnet-run listen "hey computer" \
  | while read -r detection; do aplay -q chime.wav & done
```

- **Keep the trailing `&`.** It plays the sound in the background. Without it
  the loop waits for playback, `listen` blocks writing its next line, and the
  capture tool drops audio while it waits.
- **Other players:** `pw-play chime.wav` or `paplay chime.wav` on
  PipeWire/PulseAudio, `afplay chime.wav` on macOS. The chime does not need
  to be 16 kHz: the default playback device resamples it (a raw `hw:` ALSA
  device may not).
- **The microphone hears the chime.** A short tone is safe; spoken audio from
  the speaker can mask the user or trigger a detection. Keep the sound shorter
  than `--refractory` (default: the window length, 1.5 s), or raise
  `--refractory` to cover it.
- **With `--all`**, every window is printed; pass only detections to the loop:

  ```bash
  ... | phonmatchnet-run listen "hey computer" --all \
      | grep --line-buffered '"detected":true' \
      | while read -r detection; do aplay -q chime.wav & done
  ```

`$detection` holds the detection's JSON, so the loop can also log it or pass
it on, e.g. `echo "$detection" >> detections.jsonl`.

## Limitations

- **Keyword spotting only**: no voice activity detection, endpointing or
  transcription. `listen` scores fixed windows; it does not find where the
  keyword starts or ends.
- **English only**, trained on read audiobook speech. Expect lower accuracy
  with noise, far-field microphones, accents or children.
- **No speaker verification.** Anyone saying the keyword triggers it.
- Not for security or safety-critical use.

## Development

```bash
go test ./...
go vet ./...
PHONMATCHNET_TEST_ORT=/path/to/libonnxruntime.so go test -race ./...
```

Tests that run the model skip unless `PHONMATCHNET_TEST_ORT` is set. Release
builds can set the version with
`-ldflags "-X github.com/androiddrew/phonmatchnet-run/internal/cli.version=v0.1.0"`;
`go install …@v0.1.0` records it automatically.

See LICENSE, NOTICE and [THIRD_PARTY.md](THIRD_PARTY.md). The built-in models
carry their own attribution in go-phonmatchnet. phonmatchnet-run is not
affiliated with or endorsed by NCSOFT, the PhonMatchNet authors, Google or the
g2p_en authors.
