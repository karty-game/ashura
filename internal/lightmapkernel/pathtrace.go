//go:build !js

package lightmapkernel

import _ "embed"

//go:embed wgsl/pathtrace.wgsl
var pathTraceSource string

// RNMPathTraceSource is the offline production integrator. Runtime/editor RNM
// keeps its own shader and bounded one-bounce approximation unchanged.
func RNMPathTraceSource() string { return RNMSource + "\n" + pathTraceSource }
