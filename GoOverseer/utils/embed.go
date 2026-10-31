package utils

import "embed"

// EmbeddedFS содержит встроенные ресурсы модулей демонов:
// конфигурационные схемы (*.json), шаблоны аналитики (*.html),
// скрипты визуализации (*.js) и исходный код демонов (*.go).
// Исполняемые файлы (*.exe) исключены из эмбеддинга, чтобы предотвратить
// ложные срабатывания Windows Defender и блокировки компиляции.
//
//go:embed sys/* usb/* wifi/*.json wifi/*.html wifi/*.js wifi/*.go
var EmbeddedFS embed.FS
