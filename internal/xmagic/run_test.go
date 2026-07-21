package xmagic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithoutArgumentsListsSignatures(t *testing.T) {
	var output strings.Builder
	if err := Run([]string{"xmagic"}, &output); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, value := range []string{"TYPE", "gif", "jpg", "png", "pdf", "zip"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("output = %q, want %q", output.String(), value)
		}
	}
	if strings.Contains(output.String(), "gif87a") {
		t.Fatalf("output = %q, source-only signature should not be listed", output.String())
	}
}

func TestSetReplacesKnownMagicAndPreservesSource(t *testing.T) {
	directory := t.TempDir()
	state := t.TempDir()
	t.Setenv("XMAGIC_STATE_HOME", state)
	sourcePath := filepath.Join(directory, "image.png")
	source := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("payload")...)
	if err := os.WriteFile(sourcePath, source, 0640); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := Run([]string{"xmagic", "set", "jpg", sourcePath}, &output); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	generatedPath := filepath.Join(directory, "image.jpg.png")
	generated, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0xff, 0xd8, 0xff}, []byte("payload")...)
	if !bytes.Equal(generated, want) {
		t.Fatalf("generated = %x, want %x", generated, want)
	}
	unchanged, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatal("source file was modified")
	}
	if info, err := os.Stat(generatedPath); err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("generated mode = %v, error = %v", info.Mode().Perm(), err)
	}
	if !strings.Contains(output.String(), "Replaced png magic number with jpg") {
		t.Fatalf("output = %q", output.String())
	}
	assertStateRecord(t, state, generatedPath, "replace", "png", "jpg", "89504E470D0A1A0A")
}

func TestSetPrependsMagicToUnknownFile(t *testing.T) {
	directory := t.TempDir()
	state := t.TempDir()
	t.Setenv("XMAGIC_STATE_HOME", state)
	sourcePath := filepath.Join(directory, "shell.php")
	source := []byte("<?php echo 1; ?>\n")
	if err := os.WriteFile(sourcePath, source, 0600); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := Run([]string{"xmagic", "set", "gif", sourcePath}, &output); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	generatedPath := filepath.Join(directory, "shell.gif.php")
	generated, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := append([]byte("GIF89a"), source...); !bytes.Equal(generated, want) {
		t.Fatalf("generated = %q, want %q", generated, want)
	}
	if !strings.Contains(output.String(), "Prepended gif magic number") {
		t.Fatalf("output = %q", output.String())
	}
	assertStateRecord(t, state, generatedPath, "prepend", "", "gif", "")
}

func TestSetAutomaticallyNumbersExistingOutputs(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XMAGIC_STATE_HOME", t.TempDir())
	sourcePath := filepath.Join(directory, "shell.php")
	if err := os.WriteFile(sourcePath, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "shell.gif.php"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"xmagic", "set", "gif", sourcePath}, &strings.Builder{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "shell.gif.2.php")); err != nil {
		t.Fatalf("numbered output missing: %v", err)
	}
}

func TestSetRejectsUnsupportedAndSameMagic(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XMAGIC_STATE_HOME", t.TempDir())
	sourcePath := filepath.Join(directory, "image.png")
	if err := os.WriteFile(sourcePath, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"xmagic", "set", "exe", sourcePath}, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "unknown magic-number type") {
		t.Fatalf("unsupported type error = %v", err)
	}
	if err := Run([]string{"xmagic", "set", "png", sourcePath}, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "already has png") {
		t.Fatalf("same type error = %v", err)
	}
}

func TestRunHelpAndVersion(t *testing.T) {
	var output strings.Builder
	if err := Run([]string{"xmagic", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "xmagic set <type> <file>") {
		t.Fatalf("help = %q", output.String())
	}
	output.Reset()
	if err := Run([]string{"xmagic", "--version"}, &output); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "xmagic "+Version+"\n" {
		t.Fatalf("version = %q", got)
	}
}

func assertStateRecord(t *testing.T, root, outputPath, mode, sourceType, targetType, removed string) {
	t.Helper()
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	directory := filepath.Join(root, "operations", hex.EncodeToString(digest[:]))
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("state directory missing: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("state entries = %d, want 1", len(entries))
	}
	encoded, err := os.ReadFile(filepath.Join(directory, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var record operation
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}
	if record.Mode != mode || record.SourceType != sourceType || record.TargetType != targetType || record.RemovedBytes != removed {
		t.Fatalf("state = %+v", record)
	}
}
