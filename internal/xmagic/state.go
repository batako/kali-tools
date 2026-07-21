package xmagic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func saveOperation(record operation) error {
	root, err := stateRoot()
	if err != nil {
		return err
	}
	pathHash := sha256.Sum256([]byte(record.OutputPath))
	directory := filepath.Join(root, "operations", record.OutputSHA256)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}
	content = append(content, '\n')
	temporary, err := os.CreateTemp(directory, ".operation-*")
	if err != nil {
		return fmt.Errorf("failed to create state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("failed to protect state file: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("failed to write state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("failed to sync state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close state: %w", err)
	}
	destination := filepath.Join(directory, hex.EncodeToString(pathHash[:])+".json")
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("failed to publish state: %w", err)
	}
	return nil
}

func stateRoot() (string, error) {
	if root := os.Getenv("XMAGIC_STATE_HOME"); root != "" {
		return root, nil
	}
	if root := os.Getenv("XDG_STATE_HOME"); root != "" {
		return filepath.Join(root, "xmagic"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to locate state directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "xmagic"), nil
}
