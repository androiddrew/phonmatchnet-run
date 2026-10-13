# Changelog

## [Unreleased]

### Added

- `detect AUDIO KEYWORD`: scores one 0.5–2 s mono 16 kHz 16-bit WAV clip (or `-` for stdin)
  against a typed keyword with go-phonmatchnet and prints the result as JSON.
- naptrap-compatible exit status: 0 accepted, 1 rejected, 2 error.
- `--far` operating point, `--threads`, `--model-dir`, and `--ort` / `PHONMATCHNET_RUN_ORT_LIBRARY`.
