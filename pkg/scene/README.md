# Scenes and prefabs

Scene v1 compiles strict YAML into ordinary Go factories. A prefab uses the same
format as a level: reference another file with `instance`, bind its typed inputs,
and add children or override components. YAML parsing, schema validation,
reference resolution and static transform composition happen during the build.

The portable `scene` package owns logical lifetime and component state. It has no
rendering dependency. The optional `scene/build/render3d` and `scene/render3d`
packages supply 3D authoring and runtime adapters. Future audio, 2D and game
modules can supply their own schemas and constructors through the same compiler.

This is an experimental first implementation. The
[generated-factory integration tests](build/generated_test.go) exercise authored
prefabs, generated imports and logical lifetimes without a gallery application.

## Authoring

```yaml
# yaml-language-server: $schema=../generated/scenes/scene.schema.json
version: 1
root:
  id: courtyard
  children:
    - id: power
      save_id: courtyard-power
      components:
        logic.bool: {initial: true, persistent: true}
    - id: lamp
      instance: prefabs/lamp.scene.yaml
      inputs: {power: /power}
      components:
        spatial.transform3d: {position: [4, 0, 2]}
```

The referenced `prefabs/lamp.scene.yaml` declares its input's component type:

```yaml
# yaml-language-server: $schema=../../generated/scenes/scene.schema.json
version: 1
inputs: {power: logic.bool}
root:
  id: lamp
  children:
    - id: bulb
      components:
        spatial.transform3d: {position: [0, 3, 0]}
        render3d.point:
          enabled: $power
          color: [1, 0.65, 0.25]
          intensity: 5
          range: 8
```

The compiler assembles `scene.schema.json` from exactly the installed modules;
the same schema validates the source and supports YAML IDE completion. Unknown
modules and fields, wrong types, duplicate keys, anchors, aliases, explicit tags,
merge keys, nulls, nonfinite numbers and multiple YAML documents fail the build.
Use decimal numbers, `true`/`false`, and quoted strings where necessary.

Node IDs are unique among siblings. The root node's path is `/`; child paths do
not include its authored ID. Component reference properties declare their target
type in the module. `/power` is an absolute path within the current logical root;
`../power` is relative to the referring node. `$power` uses a declared input;
nested prefabs can forward it with `inputs: {power: $power}`. Missing references,
type mismatches and constructor or file cycles fail before Go compilation.

`instance` paths are relative to the containing file and confined to the recipe's
declared source tree. Instance overrides replace a component's entire property
object, add children, and can replace the instantiated root's ID and `save_id`.
Duplicate resulting child IDs are errors. A prefab instance shares its owner's
logical root lifetime. Select a file as a recipe entry point to generate an
independent root factory; an entry point currently cannot require unbound inputs.

`spatial.transform3d` uses local XYZ positions, positive XYZ scales and XYZ Euler
angles in radians (`Rz * Ry * Rx`). The compiler composes static ancestors and
emits world transforms; unrepresentable shear is rejected. Generated transforms
are values, without a runtime parent/child propagation system.

Source limits are 2 MiB per document, 128 referenced documents, 64 nesting levels,
8,192 expanded nodes and 32,768 components. Custom modules should add their own
domain bounds. Module generators are trusted build code.

## Build integration

After prepared-resource producers in `asset.config.go`, register
`scenebuild.Scenes(scenebuild.Options{Input: ... , Modules: ...})`. Import the host
package as `scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"`. The
included module lists are `scene/build/logic.Modules()` and
`scene/build/render3d.Modules()`. See the [plugin integration tests](build/plugin_test.go).

The plugin publishes `<Package>/<Name>_gen.go` and `<Package>/scene.schema.json`
under the recipe's output directory (`Package` defaults to `scenes`). Constructors
are ordered by dependencies and expose typed component fields and a typed services
struct. Resource properties are checked against preceding producers' slot schemas.
Source dependencies, compiler changes and module configuration participate in the
existing asset cache; invalid builds preserve the last published output.

Each module supplies a strict object schema, Go field type, explicit imports and
services, typed reference/resource properties, and a Go factory expression
returning `(T, error)`. This allows application components without adding runtime
reflection or a universal component registry. See [the module API](build/types.go)
and [the boolean module](build/logic/logic.go).

## Lifetime, hooks and state

The application explicitly starts and ends a root:

```go
root, err := scenes.NewCourtyard("courtyard-1", scenes.CourtyardServices{
    Render: renderContext,
    Saves: saves,
}, func(refs *scenes.Courtyard) (func(), error) {
    stop := refs.PowerLogicBool.Subscribe(func(enabled bool) {
        // Application behavior uses typed Go references.
    })
    return stop, nil
})
// Check err, then eventually:
root.Root.Close()
```

Setup runs once after construction and restoration. Returned cleanup belongs to
the root. Cleanup runs in reverse registration order, exactly once. Constructor
or setup failure aborts construction, releases acquired resources/subscriptions
and avoids retaining newly captured save data. Handwritten behavior belongs in
Go; never edit generated factories.

`scene.Value[T comparable]` holds component-local state. `Set` synchronously
notifies ordered subscribers only when the value changes. `Subscribe` does not
emit an initial value; read `Get` when attaching a view. Unsubscribe is immediate;
a subscriber added during delivery starts with the next change. A reentrant write
to the same notifying property returns `scene.ErrReentrant`, leaving its value
unchanged. A write after its owning root closes returns `scene.ErrClosed`.
Roots, values, subscriptions and saves belong to the application update goroutine.
Dispatch asynchronous completions onto that goroutine.

Visibility, distance, mesh eviction and reacquisition do not close roots, run
setup again or reset component state. `scene/render3d.Context` owns shared model
residency; `Supply`, `Evict` and `ModelView.SetVisible` affect presentations.
Root teardown removes its presentations without evicting shared prepared assets.

## Persistence

Create one application-owned `scene.NewSaves()`. Ordinary component fields die
with their root. `logic.bool` opts in using `persistent: true` and requires an
explicit `save_id`. This ID must be unique within the authored root, including
expanded prefab instances; supply unique overrides when repeating saveable
prefabs. Saved identity combines the application's root instance ID, `save_id`
and component type. Renaming/reparenting an ordinary node does not change an
explicit saved identity.

Custom components register a typed snapshot using `scene.Persist[T]`, a version,
defaults, validation and capture/restore functions. Only the registered snapshot
is saved; callbacks, renderer objects and other component fields are excluded.
Selected values are captured on explicit export and normal root close. Close
retains them in the application service; it does not write storage automatically.

`Saves.Export` creates a versioned JSON save envelope. This serialization is
independent of authored YAML. The application chooses storage and when to save.
`Import`/`Reset` require no live saveable roots. Restore imported data before
starting roots; validate component types and versions during construction, before
setup. Importing into a fresh candidate service lets the application retain its
current scene if a load fails. Snapshots are bounded to 4 MiB and 16,384 records.
V1 rejects incompatible versions; explicit migration support remains future work.

## External assets

`scene/build/delivery.Models()` publishes prepared model bytes as immutable
`public/models/<sha256>.kmodel` payloads plus small typed Go descriptors. Upstream
producers must leave their model slots unembedded to obtain external-only delivery.
The existing GLB plugin's embedded catalog behavior is unchanged.

The CLI stages `generated/public/` into both single-sample and gallery browser
distributions. It rejects symlinks, source scenes/Go and reserved web filenames.
Authored YAML, GLBs and the IDE schema stay outside that directory and the game.

`scene/resource.Read` performs bounded, cancellable file reads or same-origin
browser fetches and SHA-256 integrity verification. Descriptors have relative
paths and a maximum payload size of 128 MiB. Browser streams abort on cancellation
or oversized responses. Call it from an I/O goroutine, then use the existing
prepared model consumer and `Context.Supply` on the update goroutine. No source
geometry conversion or image decoding is added at runtime. The existing consumer
still validates and uploads prepared resources; upload cost is not eliminated.

Applications own worker scheduling and hand completed resources back to the App
loop. Logical roots can exist before resources arrive. Delivery does not imply
an automatic spatial streaming scheduler.
SDF modeling/baking, mesh merging, automatic chunk/HLOD/impostor selection,
navigation generation and dynamic hierarchy propagation are separate future work.


## Level compiler integration

`scene/build.Resolve` exposes a privately owned, schema-validated expanded scene
for build processors. It uses the same prefab expansion, typed references,
resource admission and module semantic validation as factory compilation. Nodes
retain logical paths, source files, save IDs, ancestor component values and
concrete reference targets. Expanded snapshots, including repeated ancestor
configuration, are bounded to64MiB. `scene/build/render3d.WorldTransform` returns
the same composed transform used by generated factories and rejects shear.

`render3d.model` accepts `isDynamic: true`; omission selects static geometry for
the level bake pipeline. `render3d.point` accepts `bake: static` or
`bake: dynamic`; omission preserves its existing live enabled-state behavior.
Ordinary `Scenes()` compilation continues to generate ordinary model/light
constructors; the level compiler consumes these build annotations explicitly.

Generated baked-level factories use `scene/render3d.NewBakedModel` to preserve
logical model references whose presentation belongs to static chunks. Supplying
the original model for another dynamic instance cannot instantiate duplicate
static geometry. Such a reference never reports model residency; `SetVisible`
changes its logical view flag without editing immutable chunk geometry. Movable
or individually mutable geometry must use an explicit dynamic placement.
