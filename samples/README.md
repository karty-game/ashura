# Samples

Install [mise](https://mise.jdx.dev/installing-mise.html) 2026.11.5 or newer,
then run `mise install` from the repository root. Mise is required for all
development workflows. Start with the basic scene:

```sh
mise run start
```

Build and serve the full gallery:

```sh
mise run samples
# run only Basic Scene with TinyGo:
mise run samples-tinygo
# or
mise exec -- go run ./cmd/engine run web ./samples/...
```

Sponza uses embedded WASM libraries through Go for UVs and texture codecs, and
requires a native compute backend for lightmap baking. Mise installs the pinned
upstream GPU library when needed. No UV/encoder tool release, source compiler or
manual tool installation is required. Actual execution on each developer host
remains separately qualified. The explicit lightmap software-CI option uses
software Vulkan for functional checks.
See [Sponza's setup and qualification limits](sponza/README.md).

The CLI discovers browser-compatible `main` packages, prepares each sample's
assets and builds a gallery with a sidebar. New samples are picked up automatically.
Asset recipes run across samples with up to four workers (also capped by
`GOMAXPROCS`). Browser compilation starts after all assets are ready and batches
the sample packages into one Go build, sharing dependency discovery and Go's
normal build concurrency. Samples with the same executable name use separate
batches to keep their outputs distinct. The default `--compiler=auto` policy
uses TinyGo 0.44.0 only for Basic Scene and standard Go for every other sample.
Mixed galleries share one bridge per compiler under `runtimes/`, and each app
records its own compiler identity and bridge hash in `build-info.json`.
Explicit compiler selectors remain available for diagnostics and custom apps;
asset recipes always run with host standard Go.

Selecting a sample replaces the active view; only one scene runs at a time.
The **Open sample** link opens that sample in its own tab. Gallery selections are
also represented in the URL hash, so a selected scene can be bookmarked.

A free loopback port is selected by default (`127.0.0.1:0`). Open the printed URL;
in a remote workspace, forward that printed port or use the workspace's proxy URL.
Use `--addr 127.0.0.1:9000` when a fixed port is wanted. WebGPU needs a supported
browser and a secure context; localhost qualifies. All page, runtime and WASM URLs
are relative, including when served below a proxy path.

You can still run one sample directly:

```sh
mise exec -- go run ./cmd/engine run web ./samples/basic-scene
```

Each build is staged in a temporary directory and served only after all selected
samples compile successfully. Ctrl+C stops the server and removes its temporary
web output. Prepared assets remain in each sample's ignored `generated/assets/`.
There is no live rebuilding; rerun the command after source edits.
For a static distribution without starting a server:

```sh
mise run build-samples
# or export a single sample to a custom directory:
mise exec -- go run ./cmd/engine build web ./samples/basic-scene --output dist/basic-scene
```

`build-samples` uses the automatic compiler policy and checks the entire
published size per sample: 20,000,000 bytes maximum, with exceptions of
35,000,000 for Impostors and 45,000,000 for Sponza. The target for simple samples
is 10,000,000 bytes. Prepared assets, shared scripts and pages count before any
transfer compression. Both shared compiler runtimes count in full against every
sample. The gallery is written to `dist/samples/`, including the matching WASM runtimes,
sample pages, WASM binaries and prepared public resources. The destination must
not already exist; remove or rename a previous export before rebuilding. Failed
builds remove their staging directory without publishing a partial site. The
same `--profile` and `--no-cache` options as browser previews are supported.

The [sample workflow](../.github/workflows/samples.yml) runs the full mise check
gate (assets, formatting, native/browser lint, tests and target builds), and
exports the full gallery once on Linux amd64 for pull requests and pushes to
`main`. The same job verifies TinyGo/Go export and runtime pairing through the
CLI test suite; sample publication has no host OS matrix. After successful
checks on `main`, it deploys only
`dist/samples/` through the `github-pages` environment. It can also be run manually;
only runs on `main` deploy. In the repository's **Settings → Pages**, select
**GitHub Actions** as the build and deployment source before the first deployment.
GitHub Pages must be available for the repository's visibility and GitHub plan.
The HTTPS site works below the repository path without rewriting asset URLs.
CI tests and compilation do not establish browser GPU execution or performance.

CI restores each sample's `generated/cache/assets/v1/` action cache before asset
preparation and saves it before the check gate, so a later test failure retains
completed preparation. Cache keys include the host, pinned tools, dependencies,
recipes, source assets and processing code. A compatible older cache can seed a
changed build; the pipeline still verifies action identities, tracked inputs and
output hashes before reuse. Generated outputs, locks and recovery journals are
rebuilt locally. CI runs `assets` followed by `check-prepared`, the same checks as
`mise run check`, without a duplicate preparation pass before validation. All
checks and export budgets still run on cache hits. Sponza's GPU sampling and
resolve actions remain uncached without an explicit backend
identity; other verified preparation can be reused.
A warmed local cache does not establish a successful fresh public CI run.

Warm runs reuse Go's cached recipe executables and verified asset actions.
Unchanged generated files retain their timestamps and skip publication-journal
creation; edited or deleted generated files are restored from verified results.
Source discovery, content verification and WASM linking/staging still take time,
especially for the large Sponza sample. Changes to recipe or imported processor
code deliberately invalidate the corresponding asset action candidates.
The recipe host environment removes duplicate PATH entries while keeping their
first occurrence, so nested tool invocations do not invalidate
tool-dependent assets solely by repeating the same tool directories.

Asset recipes are demonstrated by the samples that consume their outputs:
the [material comparison](standard-materials/), [UI](ui/) and
[sprite](sprites2d/) samples cover texture preparation and atlas packing.

## Code and asset conventions

Every runnable sample follows the same lifecycle:

- `main.go` contains the window/canvas configuration and a small `main`/`run`
  pair. `main` logs a final error once, after `run` has completed its cleanup.
- `run() (err error)` creates the App, defers
  `sample.Cleanup(&err, app.Close)`, explicitly registers its GPU features with
  `app.Register`, and calls scene setup and any sample UI/diagnostics,
  and returns `app.Run(world)`. Camera/error helpers live in
  [internal/sample](internal/sample/); optional retained controls live in
  [internal/sampleui](internal/sampleui/). Application setup remains explicit.
  Basic scene omits text overlays and diagnostics to keep its rendering example
  small; its controls are documented in its README.
  UI-only samples attach their View through `uiapp.Attach` without a 3D renderer.
- `scene.go` builds the world, selects its active camera with checked errors,
  and registers its updates. Resource loading, UI construction and platform
  overlays use separate files where they would obscure that setup. Pure scene
  builders do not need an App argument. Larger scenes can return a cleanup
  function that removes their instances before unloading shared resources.
  The UI playground uses `setup.go` and an intentionally empty 3D world.
- Start material/light/probe values with the public `Default*` constructors and
  assign authored fields before attachment. Explicit zeros remain zero.

Return setup failures and unexpected runtime failures to `run`. Fallible updates
use `OnUpdateError`; fallible compute work returns its error through `OnCompute`.
When a UI/platform callback cannot return an error, pass it to a loop-owned
`sample.Errors.Check` and report `Err()` from a fallible update. Live setter and
resource-release errors are handled rather than discarded. User validation
messages are ordinary content; expected user validation failures can be displayed
locally without terminating the scene.

Samples that consume prepared source assets have:

```text
assets/          authored images/models
asset.config.go explicit build-only processing recipe
generated/      ignored prepared payloads, catalogs and processing state
README.md       usage and copyright annex for asset credits and licenses
```

Keep asset authors, source links, licenses and adaptation notes in the sample
README's `Copyright annex` section. Do not add separate sample copyright,
license, attribution or provenance files, or asset-directory notice READMEs.

`asset.config.go` owns the asset lifecycle. Before developing, prepare the named
Go declarations and their assets:

```sh
mise run assets
# Or select one project:
mise exec -- go run ./cmd/engine assets generate --project ./samples/ui
```

After developing, `engine assets build --project ./samples/ui` refreshes the
playable assets. `engine build web` and `engine run web` perform that preparation
automatically before compiling and staging the game. Both phases execute the
recipe's cached pipeline; unchanged assets are reused. `Environment.Phase`
identifies `generate` or `build` when a recipe needs phase-specific choices.
There is no sample-specific generator, downloader or source converter.
Source images are ordinary PNGs; reflection preparation belongs to the shared
`pkg/reflection/build` module.

Image-only recipes publish catalogs beneath `generated/assets`; model and
combined recipes publish beneath `generated` according to their plugins' package
layout. These are intentional output shapes of the same asset pipeline. Recipes
keep feature-specific stages, profiles and bounds: UI/sprite atlases, repeating
material maps, model preparation, impostor captures and baked levels have
different requirements. Generated files are never edited by hand.

Runtime code loads the named generated declarations. External prepared resources
use the bounded resource reader and can pass opaque verified payloads to
`model.Verified`. Source decoding, image conversion, resizing, mip generation,
geometry conversion and packing remain build work. Primitive-only scenes need
no empty asset recipe. The compute sample embeds its authored WGSL directly;
device shader compilation follows the normal compute API.

## Debug panel

Every runtime sample attaches the retained [debug panel](../pkg/debug/README.md)
through checked `debug.Show(app)`. Configure application UI before attaching it;
the panel shares that View without replacing its content. All samples start it hidden. Use **Hide / Debug**
buttons, F3 to toggle or Escape to hide. Its pinned header and scrolling metrics
use the same UI library and WebGPU pipeline on browser and native targets.
The panel uses at most 70% of the viewport height. Hide pauses metric collection
and leaves a small reopen button. There are no debug HTML controls or CSS.

Compact retained controls explain each feature. Larger comparisons collapse
advanced settings; perspective comparisons widen their field of view on narrow
viewports while keeping the camera in its authored space. The gallery, browser tabs and native windows use Ashura names.

## Sample gallery

The sidebar uses this order and these display names. New samples are still
picked up automatically and appear afterward in alphabetical order until added
to the curated order.

| Sample | What it demonstrates |
| --- | --- |
| [Basic scene](basic-scene/) | Ground, cube, sphere, hemisphere light, camera, and the App-owned game loop |
| [Materials](standard-materials/) | Standard normal/AO shading and Valve/Source RNM/SSBump, shared prepared textures and comparison controls |
| [UI Components](ui/) | WebGPU controls, actions, tabs, tooltips, alerts, progress, pagination, smooth scrolling and responsive forms |
| [Sprites2D](sprites2d/) | Prepared images and atlas regions, output-pixel placement, transforms, order, tint, opacity and clipping |
| [Billboards](billboards/) | Spherical, axis-constrained and fixed world-sized quads while orbiting |
| [Impostors](impostors/) | Complex CC0 boulder, stool and vase models beside two-triangle impostors, adaptive quality, mip controls and matched lighting |
| [Instancing](instancing/) | Five thousand moving cube instances following a two-dimensional wave, submitted with GPU instancing |
| [Reflections](reflections/) | Offline panorama bake, placed box probe with parallax-corrected cubemap sampling, reflective floor, walls and ceiling |
| [Compute texture](compute-texture/) | Embedded WGSL writes a material texture through declared compute resources and per-frame dispatch; Space pauses the pattern |
| [Sponza](sponza/) | An offline-baked atrium with RNM/SSBump relief, restored cutouts, portable textures, retained navigation/lighting controls and free flight |

A decals sample remains planned once projected decals have a supporting public
API. New examples should exercise real library functionality.
