//go:build unix

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
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file permissions %04o expose secrets; require 0600", info.Mode().Perm())
	}
	return nil
}
