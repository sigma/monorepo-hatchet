package a

import (
	"embed"
	_ "embed"
)

//go:embed testfile.txt // want "found embedded file: testfile.txt"
var content string

//go:embed *.txt // want "found embedded file: testfile.txt"
var files embed.FS
