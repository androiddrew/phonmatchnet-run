# Changelog

## [Unreleased]

### Added

- `listen KEYWORD`: watches a continuous 16 kHz mono stream (raw s16le or WAV, from standard
  input or `--input`) with a sliding window and prints one JSON line per detection.
- Debouncing with `--confirm` consecutive accepted windows and a `--refractory` period;
  `--window`, `--hop`, and `--all` to print every window's score for tuning.

## [0.1.0] - 2026-10-13

### Added

- `detect AUDIO KEYWORD`: scores one 0.5–2 s mono 16 kHz 16-bit WAV clip (or `-` for stdin)
  against a typed keyword with go-phonmatchnet and prints the result as JSON.
- naptrap-compatible exit status: 0 accepted, 1 rejected, 2 error.
- `--far` calibrated operating point or a custom `--threshold`; `--threads`, `--model-dir`, and `--ort` / `PHONMATCHNET_RUN_ORT_LIBRARY`.
