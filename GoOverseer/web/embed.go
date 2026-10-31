package web

import (
	"embed"
	"io/fs"
)

// Теперь файл лежит прямо в web/, поэтому пути к соседним папкам пишутся без точек
//
//go:embed static/* templates/*
var embedFS embed.FS

// GetStaticFS возвращает изолированную файловую систему для папки static
func GetStaticFS() fs.FS {
	sub, err := fs.Sub(embedFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// GetTemplatesFS возвращает изолированную файловую систему для шаблонов
func GetTemplatesFS() fs.FS {
	sub, err := fs.Sub(embedFS, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}
