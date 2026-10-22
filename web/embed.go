package web

import _ "embed"

//go:embed index.html
var IndexHTML []byte

//go:embed favicon.ico
var FaviconICO []byte
