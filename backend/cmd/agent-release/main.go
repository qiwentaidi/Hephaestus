// Exports the public installers without exposing source.
package main

import (
	"flag"
	"hephaestus-backend/pkg/agentrelease"
	"log"
	"os"
	"path/filepath"
)

func main() {
	output := flag.String("output", ".", "installer directory")
	flag.Parse()
	if err := os.MkdirAll(*output, 0755); err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"install.sh", "install.bat", "install.ps1"} {
		data, err := agentrelease.Installer(name)
		if err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(*output, name), data, 0755); err != nil {
			log.Fatal(err)
		}
	}
}
