package maintenance

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// Embedded firmware files for standalone flashing builds.
// Put your binaries into modes/maintenance/embedded/ before building.
//
//go:embed embedded/*
var embeddedFirmwareFS embed.FS

func readFirmwareData(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return b, nil
	}
	name := filepath.Base(path)
	if b, err := embeddedFirmwareFS.ReadFile("embedded/" + name); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("firmware not found on disk or embedded: %s", path)
}
