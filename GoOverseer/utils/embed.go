package utils

import "embed"

// EmbeddedFS содержит встроенные ресурсы всех модулей:
// JSON-схемы настроек, HTML-шаблоны, JS-скрипты аналитики,
// исходный код демонов и скомпилированные бинарники (*.exe) при их наличии.
//
//go:embed sys/* usb/* wifi/*
var EmbeddedFS embed.FS
