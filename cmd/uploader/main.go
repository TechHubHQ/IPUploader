package main

import (
	"fmt"
	"os"

	"audituploader/internal/cli"
	"audituploader/internal/log"
)

func main() {
	if err := log.InitLogger(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	code := cli.Run()
	log.Close()
	os.Exit(code)
}
