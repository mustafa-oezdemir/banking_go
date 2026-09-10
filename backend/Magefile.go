//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Build groups reproducible backend build targets.
type Build mg.Namespace

// Default builds the Linux backend binary used by docker-compose.dev.yml.
func Default() error {
	return Build{}.Linux()
}

// Linux builds one statically linked linux/amd64 API binary.
func (Build) Linux() error {
	outputPath := filepath.Join("bin", "linux", "ledger")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	env := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        "linux",
		"GOARCH":      "amd64",
	}
	return sh.RunWithV(env, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", outputPath, "./cmd")
}
