# Changelog

## [0.3.0] — 2026-10-18

### Fixed

- **Tagless model replies are no longer mangled.** The no-tag fallback let the prompt's own `Answer:` label leak into the parsed command (`Answer: tar -xzf …`), and a prose line like "Sure! Here is what you want:" beat the real command on the next line — the fallback also handed back whole prose blobs as "commands". The fallback now strips the `Answer:` label, rejects sentence-punctuation first tokens, and returns a clean `parse reply` error instead of inserting prose.
- **The destructive gate catches what the prompt already teaches.** `mv`, `truncate`, and `git push`/`git reset`/`git clean` were missing from the local safety heuristic even though the prompt's own instructions declare them unsafe — so a model that mis-tagged `git reset --hard` as safe reached the cursor with no `proceed? [y/N]` prompt on `--insert`. The local heuristic now mirrors that unsafe-verb list.
- **`ask --insert --json` fails fast** with `--insert and --json are mutually exclusive` instead of silently running insert mode — whose destructive replies read stdin, hanging a script that expected one JSON line.
- **`init` warns when your shell isn't supported.** With `$SHELL` set to fish or anything else, the block still lands in `~/.bashrc`, but init now says so on stderr instead of claiming an install your shell will never read.

### Docs

- Both READMEs document the zero-toolchain install path first (latest release tarball via the one-line `curl … | sh` installer), and `scripts/install.sh` messages state the real release matrix (darwin/amd64 + darwin/arm64 + linux/amd64) instead of stale text.

## [0.2.0] — 2026-09-12

### Fixed

- **`?`-prefix wiring works on zsh now.** The v0.1 block defined `function ?()`, and the unquoted `?` is pathname-expanded when `.zshrc` is sourced — every zsh startup aborted the block with `no matches found: ?` and the trigger never existed. bash broke too whenever the working directory contained a 1-char filename (`?` glob-expanded to it). Both shells now wire `?` through a quoted alias, which is resolved before pathname expansion.
- **Cold start can finish.** A single 30s timeout wrapped the whole `ask` flow, so the one-time 941MB model download (30-90s on typical links) always died with `context deadline exceeded`. The download/verify now runs under its own 30-minute budget; inference keeps the 30s cap.
- **`ask --json` round-trips exactly.** The reply was double-escaped (a `"` came back as `\"`, a newline as `\n`) on the machine-readable surface. Output is now produced by `encoding/json`.
- **`--no-model` is no longer a silent no-op.** It was registered but never read, so `ask --no-model` still downloaded the model. It now fails fast: `--no-model requires --mock`.
- **`init --shell` validates its input** (`zsh`/`bash` only — `--shell fish` used to silently write the block into whatever rc was detected), the zsh override honours `$ZDOTDIR`, and `--dry-run` previews the exact per-shell block instead of a stale copy.

### Added — m2_shell_meet

- **`ask --insert`**: prints the command only, ready for `$(...)` capture by the shell function. Destructive replies gate on a `proceed? [y/N]` prompt read from stdin — anything other than `y`, including EOF, declines with no output (fail closed).
- **Cursor insertion + Ctrl-Space**: zsh `?` inserts the command at the cursor via `print -z`, and Ctrl-Space (`^@`) rewrites the current line via a ZLE widget; bash `?` prints for copy (a plain bash function cannot edit the readline buffer) and Ctrl-Space rewrites the line via `bind -x` + `READLINE_LINE`. Insert-only — you still press Enter.

### Housekeeping

- Version surfaces bumped to 0.2.0 (`VERSION`, the `main.version` dev constant, `web/site.json` meta.content_version), a version-consistency test pins the lockstep, and `docs/demo.tape` now demonstrates the v0.2 insert path (the committed demo gif stays a v0.1.0 recording; re-render it via the manual demo workflow).

## [0.1.0] — 2026-08-18

Initial release: `sh1llwrt ask` turns a sentence into a shell command offline via llama.cpp + Qwen2.5-Coder-1.5B (Q4_K_M, 941MB, sha256-verified one-time download), `sh1llwrt init` wires the `?`-prefix, curl|sh installer, and goreleaser binaries for darwin/arm64 + linux/amd64.
