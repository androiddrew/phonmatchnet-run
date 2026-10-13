# Attribution and terms

Original code is Apache-2.0 (LICENSE/NOTICE).

| Component | Source/version | Terms |
|---|---|---|
| go-phonmatchnet, with its embedded models | github.com/androiddrew/go-phonmatchnet v0.1.0 | Apache-2.0; embedded models carry their own attribution in that module's NOTICE and THIRD_PARTY.md (Google, g2p_en, NCSOFT BSD-3-Clause training code, LibriSpeech CC BY 4.0) |
| cobra | github.com/spf13/cobra v1.10.2 | Apache-2.0 |
| pflag | github.com/spf13/pflag v1.0.9 | BSD-3-Clause |
| onnxruntime_go | v1.22.0 | MIT |
| ortenv | github.com/androiddrew/ortenv v0.2.0 | MIT |
| golang.org/x/text | v0.41.0 | BSD-3-Clause |

A binary built from this repository contains the go-phonmatchnet models;
redistribute it with that module's NOTICE and THIRD_PARTY.md.

Test fixtures (`testdata/`, not part of the binary):

| Files | Source | Terms |
|---|---|---|
| `testdata/said-mister.wav` | LibriSpeech utterance 6188-78885-0016 (train-clean-360), samples 3040-11680, via go-phonmatchnet | CC BY 4.0 |
| `testdata/af_heart-hey-commuter.wav` | Synthesized with Kokoro-82M v1.0 (voice `af_heart`), via go-phonmatchnet | Kokoro-82M is Apache-2.0 |
