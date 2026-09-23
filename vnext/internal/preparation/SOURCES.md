# Dependency preparation sources

Recipes were checked on 2026-09-22. Version probing establishes CLI availability only; Beads backend initialization and Serena MCP/language-server qualification remain separate operations. No setup, hook, shell-profile or MCP-registration command is run by this package.

## Beads

- [Official v1.3.0 release](https://github.com/gastownhall/beads/releases/tag/v1.3.0) and [release asset manifest](https://api.github.com/repos/gastownhall/beads/releases/tags/v1.3.0).
- Fixed download base: `https://github.com/gastownhall/beads/releases/download/v1.3.0/`.
- Asset names are `beads_1.3.0_<os>_<arch>.tar.gz`, or `.zip` on Windows. The same release supplies `checksums.txt`; SHA-256 is verified before extracting the single `bd`/`bd.exe` file. Downloads and extracted bytes are bounded. No archive paths are written verbatim.
- [Release build configuration](https://github.com/gastownhall/beads/blob/v1.3.0/.goreleaser.yml) names the binary `bd`. Linux amd64/arm64 and Windows amd64 builds have CGO enabled. Windows arm64, Android and FreeBSD builds can require an external Dolt backend; availability never claims backend readiness.
- [Installation documentation](https://github.com/gastownhall/beads/blob/v1.3.0/docs/getting-started/installation.md) also documents source installation. That is manual guidance only: the automated recipe never installs Go or a compiler.
- `bd version` probes the selected executable. `Initialize` separately runs `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing` only after explicit selection and approval, setting `BEADS_DIR` to the project directory. These flags are defined by the [tagged init command](https://github.com/gastownhall/beads/blob/v1.3.0/cmd/bd/init.go). Existing recognizable metadata is reused. Both new and existing configurations must pass `bd --readonly status --json --no-activity` and return a valid issue summary before being marked prepared; the [tagged status implementation](https://github.com/gastownhall/beads/blob/v1.3.0/cmd/bd/status.go) propagates backend statistics errors. Transaction/coordination capabilities remain separate gates. No `bd setup` command runs.

## Serena and Graphify

- [Serena upstream quick start](https://github.com/oraios/serena#quick-start) identifies `serena-agent` and `uv tool install -p 3.13`; [official registry release](https://pypi.org/pypi/serena-agent/1.7.0/json) supplies pin `1.7.0`. The [tagged CLI](https://github.com/oraios/serena/blob/v1.7.0/src/serena/cli.py) supports `--version`.
- [Graphify upstream install instructions](https://github.com/Graphify-Labs/graphify#install) explicitly identify `graphifyy` (double y). [Official registry release](https://pypi.org/pypi/graphifyy/0.9.65/json) supplies pin `0.9.65`. The published wheel's `graphify/__main__.py` supports `--version`.
- Fixed recipes: `uv tool install --no-config --python 3.13 serena-agent==1.7.0` and `uv tool install --no-config --python 3.13 graphifyy==0.9.65`.
- [uv environment documentation](https://docs.astral.sh/uv/reference/environment/) defines `UV_TOOL_DIR`, `UV_TOOL_BIN_DIR`, `UV_CACHE_DIR`, `UV_PYTHON_DOWNLOADS`, `UV_PYTHON_INSTALL_DIR`, `UV_PYTHON_BIN_DIR`, `UV_PYTHON_INSTALL_BIN`, and `UV_PYTHON_INSTALL_REGISTRY`. Approved installs reuse existing uv, or bootstrap the pinned official binary below. Python 3.13 can be downloaded by uv into `.agent-team/dependencies/python`; Python executable links, tool environments, executables and cache also stay under `.agent-team/dependencies`. `UV_PYTHON_INSTALL_BIN=false`, `UV_PYTHON_INSTALL_REGISTRY=false`, and `UV_NO_MODIFY_PATH=1` disable default Python command installation, Windows registry changes, and PATH changes. Direct packages are pinned; this is not a complete transitive dependency lock.
- Missing uv is bootstrapped from the [official 0.12.17 release](https://github.com/astral-sh/uv/releases/tag/0.12.17), using SHA-256 pins from its [release asset manifest](https://api.github.com/repos/astral-sh/uv/releases/tags/0.12.17). Only the `uv`/`uv.exe` executable is extracted, under the project dependency directory; no upstream installer script runs. Linux/macOS and Windows amd64/arm64 archives are supported. Existing scoped uv is preferred before PATH lookup.
- Installation never runs `graphify install`, Graphify hook commands, Serena init or host registration.

## Project initialization

`Initialize` is separately consent gated and never installs tools. Missing optional tools retain native fallback guidance. Unknown or redirected project state is preserved.

Existing Serena YAML is parsed with the YAML organization's security-maintained
[`go.yaml.in/yaml/v3` v3.0.5](https://github.com/yaml/go-yaml/releases/tag/v3.0.5),
pinned in `go.mod` and `go.sum`. This is an intentional native library dependency:
configuration syntax and scalar/list types require a YAML parser, while reused
Serena installations need not expose a discoverable Python interpreter. Reads are
limited to 64 KiB; custom fields are accepted and never rewritten. The compiled
module appears in Go build information; release module provenance should include
the pin alongside the existing file-oriented SBOM.

Empty source projects and projects without a committed HEAD report `deferred`,
with CLI availability preserved and `Prepared=false`. Planning can proceed;
required capability gates still require preparation before dispatch. Failed
Serena creation records a complete digest of its generated partial directory;
an approved retry isolates only an unchanged matching partial tree. Customized
or unknown output remains untouched.

Graphify also defers committed empty or document-only inventories before consent
or extraction. The conservative document suffix list comes from pinned 0.9.65
`graphify/detect.py` (`.md`, `.mdx`, `.qmd`, `.skill`, `.txt`, `.rst`, `.html`).
Unknown files, scripts and package manifests still reach the extractor, so actual
extraction failures are retained rather than reclassified as empty projects.

An approved attempt that receives a clear unsupported command/option/flag error
from a global CLI records its exact path/version (plus executable digest when
bounded). A later approved install can select a pinned project-local copy.
Generic project/configuration errors do not request reinstall, and shared CLI
files are never overwritten.

Approved installs and project initialization share a recoverable mutation guard
under `.agent-team/dependencies/coordination`. It uses the store's exact owner
token and native process-identity liveness proof before recovering abandoned
primary or recovery locks. Live or unverifiable owners are preserved. Legacy
empty `.install-lock` and `.graphify-lock` markers are preserved but no longer
control preparation; no process is killed to recover a lock.

- Serena: `serena project create <root> --name <basename> --language <language> ...`. A bounded scan selects recognized source extensions and passes explicit supported language IDs, avoiding the upstream CLI's interactive inference prompt on mixed-language projects. Generated dependency/build folders are excluded; empty or unsupported source projects receive actionable guidance. The [v1.7.0 CLI](https://github.com/oraios/serena/blob/v1.7.0/src/serena/cli.py) defines these options and the [language-server enumeration](https://github.com/oraios/serena/blob/v1.7.0/src/solidlsp/ls_config.py) defines their values. The [configuration implementation](https://github.com/oraios/serena/blob/v1.7.0/src/serena/config/serena_config.py) supports `SERENA_HOME`. Initialization sets that variable to `.agent-team/dependencies/serena-home`, containing generated global config and project registration inside the project. Existing recognizable `.serena/project.yml` is reused. No language-server indexing or MCP registration runs.
- Graphify: `graphify extract . --code-only --no-viz`, documented by the [upstream README](https://github.com/Graphify-Labs/graphify) and published `graphifyy` CLI. Extraction runs from the source project, writing to a unique staging directory under `.agent-team/dependencies/prepared` via explicit `GRAPHIFY_OUT`. Receipt schema 2 in `.agent-team/dependencies/prepared/graphify.json` binds the graph and entire output tree to the project, HEAD, executable path/version, and current tracked/untracked nonignored source-file bytes. Generated `.agent-team`, `.beads`, `.serena`, and `graphify-out` state is excluded from source fingerprints. Dirty or untracked source edits invalidate reuse even if HEAD is unchanged; approved preparation refreshes the graph. Source fingerprints record symlink target text without following external targets, plus gitlink object IDs and checked-out submodule sources. Each source tree is bounded to 50,000 entries and 256 MiB, nesting to eight submodules, and the overall fingerprint deadline to 30 seconds. Symlinks, special files, and altered **output** sidecars prevent reuse or replacement. HEAD and source bytes are rechecked before publishing; prior output survives extraction failures. Pinned upstream manifest/cache entries refer to the source root, so moving staged output preserves their meaning. Temporary staging files are removed.
- `Prepared` describes project artifacts, not backend transactions, language-server operations or fresh-worker capability inheritance. Those remain independent workload checks.

## Other optional executables

`rg`, `ast-grep`, and `lean-ctx` are inspected and reused. After approval, missing tools are downloaded as fixed official release archives into the project dependency directory. SHA-256 digests are pinned in `tools_install.go` from the official release asset manifests checked on 2026-09-22. Downloads are bounded to 128 MiB; extracted executables and TAR decompression are bounded to 256 MiB. Only the exact executable archive member is extracted; archive paths never become filesystem destinations. Existing files are preserved with exclusive creation, and executable availability is probed after installation. Unsupported platforms and failed probes retain explicit fallback guidance.

- ripgrep [15.2.0 release](https://github.com/BurntSushi/ripgrep/releases/tag/15.2.0), [asset digests](https://api.github.com/repos/BurntSushi/ripgrep/releases/tags/15.2.0), and [installation documentation](https://github.com/BurntSushi/ripgrep#installation). Linux amd64/arm64 use musl archives; macOS and Windows amd64/arm64 use their native archives. The executable is `ripgrep-15.2.0-<target>/rg` (or `rg.exe`).
- ast-grep [0.45.3 release](https://github.com/ast-grep/ast-grep/releases/tag/0.45.3), [asset digests](https://api.github.com/repos/ast-grep/ast-grep/releases/tags/0.45.3), and [installation documentation](https://ast-grep.github.io/guide/quick-start.html#installation). Linux/macOS/Windows amd64/arm64 use `app-<target>.zip`. Linux builds require a compatible GNU libc runtime. Only `ast-grep`/`ast-grep.exe` is extracted, avoiding Linux's unrelated `sg` executable.
- LeanCTX [v3.10.2 release](https://github.com/yvgude/lean-ctx/releases/tag/v3.10.2), [asset digests](https://api.github.com/repos/yvgude/lean-ctx/releases/tags/v3.10.2), and [upstream documentation](https://github.com/yvgude/lean-ctx). Linux amd64/arm64 use musl archives; macOS amd64/arm64 and Windows amd64 use their native archives. Upstream publishes no Windows arm64 binary for this release, so it receives explicit platform guidance. Only `lean-ctx`/`lean-ctx.exe` is extracted. No `wrap`, `onboard`, setup or proxy activation is performed.
