//go:build !unix

package config

import (
	"fmt"
	"os"
)

func CheckPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect config file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config path must resolve to a regular file")
	}
	return nil
}
