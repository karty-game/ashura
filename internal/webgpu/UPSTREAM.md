# Retained WebGPU binding

This is the `wgpu` package from `github.com/go-webgpu/webgpu` v0.5.5
(commit `86cc998b4641d1ad89f65bc48a155a989f76acd4`), retained under its MIT license in `LICENSE`. Rendering and
offline baking import this same package. It is internal so the corrected code
ships with Ashura to downstream Go modules; a main-module-only `replace` would
not propagate to those consumers. No native binary or second binding is included.

Changes from upstream:

- Describe C by-value callback and free-member aggregates to GoFFI on Unix.
- Copy adapter-info strings before native allocations are freed.
- Include v29's binding-array field and padding in bind-group layout entries.
- Provide explicit library initialization without process environment mutation,
  retain the selected library identity and reject switching a live binding.
- Restrict this native package to non-JavaScript targets. Use the existing
  purego callback-runtime provider with `-tags=nofakecgo` in native builds.
- Admit depth/stencil-only render passes with a null color-attachment pointer,
  while rejecting empty passes and missing/released depth views.

GoFFI is the upstream v0.6.8 module. Its released argument-layout and stack-frame
fixes replace the previous investigation patch; no GoFFI fork is retained.
Specific ABI/string regressions live beside the binding; hardware execution is
covered through the production baker. Original upstream graphics tests are not
copied into the default source gate because they initialize native GPU devices
unconditionally. The remaining renderer-wide ABI gaps documented upstream are
outside this bake migration; native renderer hardware coverage remains explicit.

Upstream files keep their source layout and are excluded from Ashura's style
formatter/linter rules. They still compile in native builds and their regression
tests run with the normal Go test task. Updates must preserve these corrections
until an upstream release includes equivalent fixes.
