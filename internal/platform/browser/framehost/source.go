// Package framehost supplies the static browser frame host used by exported
// application pages. It owns no GPU implementation or compiler runtime bridge.
package framehost

import _ "embed"

// Source installs __ashuraCreateFrameHost before the application starts. Custom
// browser hosts must load this same source before running the WASM application.
//
//go:embed framehost.js
var Source string
