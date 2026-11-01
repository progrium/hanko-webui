// Package hankoid exposes repository-level files to the commands.
package hankoid

import _ "embed"

// License is the full text of this project's LICENSE (GNU AGPLv3).
//
//go:embed LICENSE
var License string

// SourceURL is where the corresponding source code is published.
const SourceURL = "https://github.com/progrium/hankoid"
