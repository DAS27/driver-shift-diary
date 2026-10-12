// Package web contains the browser client embedded into the server binary.
package web

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
