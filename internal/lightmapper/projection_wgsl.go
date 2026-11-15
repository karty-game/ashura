package lightmapper

import _ "embed"

// CalibrationWGSL shares fixed projection kernels with the offline compute executor.
//
//go:embed wgsl/calibration.wgsl
var CalibrationWGSL string
