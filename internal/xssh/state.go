package xssh

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type credentialState interface {
	Load() (int64, error)
	Save(id int64) error
}

type noopCredentialState struct{}

func (noopCredentialState) Load() (int64, error) {
	return 0, nil
}

func (noopCredentialState) Save(int64) error {
	return nil
}

type fileCredentialState struct{}

func (fileCredentialState) Load() (int64, error) {
	path, err := credentialStatePath()
	if err != nil {
		return 0, err
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64)
	if err != nil || id < 1 {
		return 0, nil
	}
	return id, nil
}

func (fileCredentialState) Save(id int64) error {
	if id < 1 {
		return nil
	}
	path, err := credentialStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.FormatInt(id, 10)+"\n"), 0600)
}

func credentialStatePath() (string, error) {
	root := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(root, "xssh", "state"), nil
}
