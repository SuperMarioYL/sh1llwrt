[简体中文](./README.zh-CN.md) · [Website](https://sh1llwrt.lei6393.com) · [GitHub](https://github.com/SuperMarioYL/sh1llwrt)

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/hero-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/hero-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/hero-dark.svg">
  <img src="./assets/presentation/hero-light.svg" width="960" alt="Hero diagram">
</picture>

# sh1llwrt

**Read the proposed shell command before running it.**

sh1llwrt combines a request with shell, working-directory and filename context, asks a local model for a structured reply, and prints the proposed command with an explanation.

## Why use it

A command suggestion is more useful when it knows the shell and names you are working with. Keep that context in the prompt and return a command you can review at the terminal.

- **Include shell context** — The request carries shell, cwd and named files.
- **Review structured output** — JSON retains the command and explanation.
- **Try the interface offline** — Mock mode exercises parsing without model setup.

## Architecture

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/architecture-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/architecture-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/architecture-dark.svg">
  <img src="./assets/presentation/architecture-light.svg" width="960" alt="Architecture diagram">
</picture>

BuildPrompt constructs a shell-scoped request. The model cache prepares the GGUF for CLIRunner, which invokes an external llama-cli process; ParseReply extracts command, safe and explain fields. Print or JSON integrators display the result. --mock supplies deterministic responses through the same parser.

| Component | Responsibility |
| --- | --- |
| `Request context` | internal/prompt/template.go |
| `GGUF cache` | internal/model/cache.go |
| `CLI or mock runner` | internal/llama/runner.go |
| `Reviewable output` | internal/shell/integration.go |

## Install and quickstart

Use the runtime version declared in the repository manifest. The source installation below makes the included example reproducible.

```bash
git clone https://github.com/SuperMarioYL/sh1llwrt.git
cd sh1llwrt
go build ./cmd/sh1llwrt
```

Go 1.24+ and Python 3; two complete mock requests produce structured proposals without executing them.

```bash
python3 examples/presentation_demo.py
```

## Recorded demo

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/process-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/process-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/process-dark.svg">
  <img src="./assets/presentation/process-light.svg" width="960" alt="Process diagram">
</picture>

The fixture returns a tar extraction proposal marked false and a pod-listing proposal marked true; neither command runs.

```text
{"command":"tar -xzf foo.tar.gz -C /tmp","safe":false,"explain":"extract gzipped tarball into /tmp"}
{"command":"kubectl get pods -n kube-system","safe":true,"explain":"read-only pod listing"}
```

The complete command and output are recorded in [docs/demo-results.json](./docs/demo-results.json). Inputs and reproduction code are included in the repository.

![Existing terminal recording](./assets/demo.gif)

The existing recording is retained for context; the text example above documents the reproducible scenario.

## Usage

Run these commands from the repository root after installation. Replace paths for your own data.

```bash
go run ./cmd/sh1llwrt ask --mock --json "list kube-system pods"
# With llama-cli and the model available:
go run ./cmd/sh1llwrt ask --shell bash --cwd "$PWD" --glob clip.mov "convert clip.mov to mp4 h264"
```

## Configuration

SH1LLWRT_CACHE_DIR chooses the GGUF cache; SH1LLWRT_MODEL_URL and SH1LLWRT_MODEL_SHA256 select its source and expected digest; SH1LLWRT_LLAMA_CLI selects the executable. --threads, --max-tokens (96 by default) and --temperature control generation. --glob supplies named files rather than reading arbitrary file content. init writes shell integration; the quickstart demo does not modify shell configuration.

## Integrations and responsibilities

<picture>
  <source media="(max-width: 600px) and (prefers-color-scheme: dark)" srcset="./assets/presentation/integrations-mobile-dark.svg">
  <source media="(max-width: 600px)" srcset="./assets/presentation/integrations-mobile-light.svg">
  <source media="(prefers-color-scheme: dark)" srcset="./assets/presentation/integrations-dark.svg">
  <img src="./assets/presentation/integrations-light.svg" width="960" alt="Integrations diagram">
</picture>

Choose the input and output route that matches your workflow. The local example below exercises the stated subset.

| Route | Implemented role |
| --- | --- |
| Bash / Zsh context | Prompt grammar selection |
| GGUF file | Local model cache |
| llama-cli | External inference executable |
| Text / JSON | Command proposals |
| Mock backend | No-model examples |

## Limits and next steps

- The recorded proposals come from keyword-based mock responses. They do not measure model quality, warm-cache latency or hardware performance.
- The safe flag is a supplied classification, not an independent safety check. The CLI prints suggestions and does not execute them; --insert is not implemented.
- Real inference needs a separately installed llama.cpp CLI and the GGUF. The built-in expected SHA256 is empty; checksum verification requires SH1LLWRT_MODEL_SHA256.

Inline cursor insertion, packaged inference runtime and a representative command-quality benchmark remain future work.

## License and contributions

See [LICENSE](./LICENSE). When reporting an issue, include a minimal input, the command, and the observed output.
