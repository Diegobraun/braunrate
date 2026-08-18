// Package examples carries the published scenarios into the binary so the
// interface can show them without the repository being on disk. They are the
// same files the CLI and the gate run, embedded, not copied.
package examples

import "embed"

//go:embed *.yaml
var Files embed.FS
