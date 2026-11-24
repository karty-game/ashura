# Contributor instructions

Read `README.md` and `spec/README.md` before changing the library. Preserve
unrelated work. Do not commit or publish automatically. Leave local `.codex/`
and `.agents/` directories alone.

## Layout and ownership

- Public Go packages live under `pkg/`; imports include `/pkg/`.
- Private implementation lives under `internal/`, commands under `cmd/`, and
  maintained examples under `samples/`. Keep branding in `branding/`.
- `spec/` is the maintained source for contracts, proposals and validation.
  Update its index and relevant specifications when changing behavior.
  User-facing asset pipeline guides live in `docs/asset-pipeline/`; keep them
  aligned with plugin options and effective defaults. Group them as importers,
  processors, generators and publishers; use domain prefixes such as `image-` or `model-`
  in plugin filenames. Do not duplicate contracts
  there or create a root `API.md`.
- Unavoidable third-party native asset tools belong in the external tool/plugin
  distribution. See `spec/asset-tools.md` for module integration and release availability. Required
  mise installs pinned prebuilt packages; do not recreate a downloader, archive
  installer or consumer source-build fallback. Maintained tools require Linux
  amd64/arm64, macOS arm64 and Windows amd64 qualification and complete releases,
  or a qualified portable implementation. Native source compilation belongs only
  to tool-maintainer release workflows. Never install tools during game loading.
- Keep generated sample assets ignored. Change their generators and recipes,
  then regenerate; do not edit generated catalogs by hand.
- Local investigations belong in ignored `tmp/` or `spikes/`. Preserve the
  `tmp/go.mod` and `dist/go.mod` boundaries.

## Portable tooling

- Prefer Go for scripting and automation, including local helpers, generators,
  validation and CI. Avoid introducing Node.js or Python as scripting runtimes.
  Existing browser JavaScript and browser-test infrastructure do not justify
  adding either runtime to unrelated tooling.
- Write project-owned CLI and tool logic in Go. Discourage Rust or C/C++ helper
  executables, workers and generators when Go or Go-hosted WASM/WASI can perform
  the work. Moving such tools to another repository does not resolve their
  portability and maintenance costs.
- Prefer reusable, checksum-pinned WASM/WASI libraries executed directly in Go
  through the existing wazero integration over repackaged native CLIs. Keep
  orchestration, validation, caching and publication in Go; preserve licenses,
  memory bounds, cancellation and complete output admission. Do not retain
  native fallback or qualification paths for processors already provided by
  these portable libraries.
- Keep exceptions narrow and document the concrete Go/WASM capability gap and
  supported-host requirements. Existing native GPU/windowing libraries and the
  Materialize CLI are specific dependencies, not a precedent for new native
  tools or scripting runtimes. Consumers must not need additional native
  compilers or per-platform tool releases when a portable implementation works.

## Toolchain and validation

Keep CI focused and fast. Add validation steps, gates, jobs or matrices only
when explicitly requested or approved by the user. One-off investigations,
temporary regressions and diagnostic exports stay local in ignored `tmp/` or
`spikes/`; do not promote them into CI automatically. Avoid duplicate asset
preparation and exports; use the existing checks and verified caches.
Run whole-tree formatting and native/browser engine/library lint before asset
preparation. Sample type lint follows preparation because samples import their
generated catalogs. Keep those lint scopes separate to avoid checking them twice.

Mise >=2026.11.5 is required. Use it directly from the repository root: `mise run <task>` for maintained
tasks and `mise exec -- <command>` for focused commands. Do not add toolchain
wrappers. Use the pinned standard Go compiler for native builds and offline asset recipes.
Browser apps may use the pinned Go or TinyGo compiler through the CLI selector;
ship the matching compiler runtime bridge with each WASM artifact.
The default sample policy uses TinyGo only for Basic Scene and standard Go for
every other sample. Mixed galleries retain separate compiler runtimes and
per-application compiler receipts; never substitute one runtime for the other.
Published sample budgets apply per platform and count logical file lengths:
WASM, every common asset, the selected prepared texture profile and shared
host/runtime files. Separate external profiles use `texture-profiles/bc/` and
`texture-profiles/mobile/`; every other exported file counts against both.
The complete gallery report also counts all platform variants together. Ordinary samples must
stay within 20,000,000 bytes; Impostors may use 35,000,000 bytes and Sponza
45,000,000 bytes. Moving assets
outside WASM or quoting compressed transfer sizes does not satisfy these limits.
Sample exports enforce complete-file budgets before publication. Run
`build-samples` for the release gallery; see decision 0017.
GPU implementations must be explicitly registered before Run; do not add an
implicit all-feature preset or frame-time constructor fallback. Keep optional
UI/text shaping out of core engine/platform dependencies and Basic scene.
Prove dependency and artifact exclusions when changing feature boundaries;
runtime enable flags alone do not establish smaller builds. See decisions 18–19.
For agent shell commands, use a non-login shell (`login: false`): the workspace
login profile can put another Go compiler ahead of mise. Verify both
`mise exec -- go version` and `mise exec -- sh -c 'go version'` match the pin.

Check cache access before Go workflows. Reuse writable warmed caches, or use
`/tmp/karty-webgpu-go-cache` for `GOCACHE` and
`/tmp/karty-webgpu-lint-cache` for `GOLANGCI_LINT_CACHE`. Serialize full-module
tests, builds and lint across repositories. Run `mise run check` for module
changes and record commands, outcomes and limits in `spec/validation.md`.

Asset transformations belong offline. New asset-loading or rendering work needs
explicit user approval before implementation. Preserve admission bounds,
checksums, path confinement, ownership and allocation constraints.
Every maintained sample must be portable beyond BC-only devices. Prepare native
mobile texture variants offline and select a supported profile after device
initialization. Preserve independent RNM/SSBump coefficients; two-channel normal
presets and reconstructed Z are unsuitable for those numeric maps.

Local graphics review requires hardware GPU rendering and verified adapter
identity. Report blocked hardware checks rather than silently using software.
Compilation, WASM execution, GPU pixels and device performance are separate claims.
