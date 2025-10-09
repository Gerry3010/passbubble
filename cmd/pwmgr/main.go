package main

import (
	"log"
	"os"

	"github.com/gerry/password-manager/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
