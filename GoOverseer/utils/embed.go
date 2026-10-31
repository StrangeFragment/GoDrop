package utils

import "embed"

// EmbeddedFS содержит встроенные ресурсы всех модулей демонов:
// конфигурационные схемы (*_info.json), скрипты аналитики, HTML-шаблоны
// и скомпилированные бинарники демонов (*.exe).
//
//go:embed sys/* usb/* wifi/*
var EmbeddedFS embed.FS
