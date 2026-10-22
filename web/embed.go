package web

import _ "embed"

// IndexHTML — главная страница (шаблон html/template).
//
//go:embed index.html
var IndexHTML []byte

//go:embed favicon.ico
var FaviconICO []byte

//go:embed icon-192.png
var Icon192 []byte

//go:embed icon-512.png
var Icon512 []byte

//go:embed apple-touch-icon.png
var AppleTouchIcon []byte

// Manifest — манифест веб-приложения: при добавлении ярлыка
// на телефон используется новая иконка, а не белый квадрат.
const Manifest = `{
  "name": "GoDrop!",
  "short_name": "GoDrop",
  "description": "Передача файлов с телефона на компьютер",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#0f0f1a",
  "theme_color": "#0f0f1a",
  "icons": [
    {
      "src": "/icon-192.png",
      "sizes": "192x192",
      "type": "image/png",
      "purpose": "any"
    },
    {
      "src": "/icon-512.png",
      "sizes": "512x512",
      "type": "image/png",
      "purpose": "any"
    }
  ]
}
`
