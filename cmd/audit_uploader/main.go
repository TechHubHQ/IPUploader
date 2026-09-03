package main

import (
	"audituploader/internal/composer"
	"audituploader/internal/log"
)

func main() {
	if err := log.InitLogger(); err != nil {
		panic(err)
	}
	composer.Run()
}
