# phonmatchnet-run

A command-line **text-defined keyword detector**. Give it a short audio clip
and a keyword typed as text, and it prints the probability that the keyword was
spoken. New wake words need no recordings or training.

It runs naptrap's independently trained English
[PhonMatchNet](https://huggingface.co/androiddrew/naptrap-phonmatchnet-en)
through [go-phonmatchnet](https://github.com/androiddrew/go-phonmatchnet). The
models are built into the binary; the only external requirement is an
installed ONNX Runtime. The command follows `naptrap detect`.

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

## Limitations

- **Single clips, not a streaming listener.** To watch a microphone, window the
  audio yourself (e.g. 1.5 s windows every 0.25 s) and debounce repeated accepts.
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
