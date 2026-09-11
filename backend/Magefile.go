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

// NotificationLinux builds the standalone Notification service for linux/amd64.
func (Build) NotificationLinux() error {
	outputPath := filepath.Join("bin", "linux", "notification-service")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	env := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        "linux",
		"GOARCH":      "amd64",
	}
	return sh.RunWithV(env, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", outputPath, "./cmd/notification-service")
}

// IdentityLinux builds the standalone Identity service for linux/amd64.
func (Build) IdentityLinux() error {
	return buildLinux("identity-service", "./cmd/identity-service")
}

// GatewayLinux builds the public edge gateway for linux/amd64.
func (Build) GatewayLinux() error {
	return buildLinux("gateway", "./cmd/gateway")
}

func buildLinux(name, packagePath string) error {
	outputPath := filepath.Join("bin", "linux", name)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	return sh.RunWithV(map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"}, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", outputPath, packagePath)
}
