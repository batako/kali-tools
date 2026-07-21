package xmagic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var Version = "1.0.0"

const usageText = `usage: xmagic [ls]
       xmagic set <type> <file>

Create a copy whose leading magic number identifies it as another file type.
The source file is never modified.

commands:
  ls                    list supported magic-number types
  set <type> <file>     replace a known magic number or prepend one if unknown

types:
  gif                   GIF89a
  jpg                   JPEG (alias: jpeg)
  png                   PNG
  pdf                   PDF
  zip                   ZIP

options:
  -h, --help            show this help
  -V, --version         show version`

type signature struct {
	Name        string
	Aliases     []string
	Description string
	Bytes       []byte
	SourceOnly  bool
}

var signatures = []signature{
	{Name: "gif", Aliases: []string{"gif89a"}, Description: "GIF89a image", Bytes: []byte("GIF89a")},
	{Name: "jpg", Aliases: []string{"jpeg"}, Description: "JPEG image", Bytes: []byte{0xff, 0xd8, 0xff}},
	{Name: "png", Description: "PNG image", Bytes: []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}},
	{Name: "pdf", Description: "PDF document", Bytes: []byte("%PDF-")},
	{Name: "zip", Description: "ZIP archive", Bytes: []byte{'P', 'K', 0x03, 0x04}},
	{Name: "gif87a", Description: "GIF87a image", Bytes: []byte("GIF87a"), SourceOnly: true},
}

type operation struct {
	Version      int    `json:"version"`
	Mode         string `json:"mode"`
	SourcePath   string `json:"source_path"`
	OutputPath   string `json:"output_path"`
	SourceType   string `json:"source_type,omitempty"`
	TargetType   string `json:"target_type"`
	RemovedBytes string `json:"removed_bytes,omitempty"`
	AddedBytes   string `json:"added_bytes"`
	SourceSHA256 string `json:"source_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	CreatedAt    string `json:"created_at"`
}

func Run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		args = []string{"xmagic"}
	}
	if len(args) == 1 {
		return listSignatures(stdout)
	}

	switch args[1] {
	case "ls":
		if len(args) != 2 {
			return errors.New("usage: xmagic ls")
		}
		return listSignatures(stdout)
	case "set":
		if len(args) != 4 {
			return errors.New("usage: xmagic set <type> <file>")
		}
		return setMagic(args[2], args[3], stdout)
	case "-h", "--help", "help":
		if len(args) != 2 {
			return errors.New("usage: xmagic --help")
		}
		_, err := fmt.Fprintln(stdout, usageText)
		return err
	case "-V", "--version", "version":
		if len(args) != 2 {
			return errors.New("usage: xmagic --version")
		}
		_, err := fmt.Fprintf(stdout, "xmagic %s\n", Version)
		return err
	default:
		return fmt.Errorf("unknown xmagic command: %s", args[1])
	}
}

func listSignatures(stdout io.Writer) error {
	if _, err := fmt.Fprintln(stdout, "TYPE  MAGIC                    DESCRIPTION"); err != nil {
		return err
	}
	for _, item := range targetSignatures() {
		if _, err := fmt.Fprintf(stdout, "%-5s %-24s %s\n", item.Name, formatHex(item.Bytes), item.Description); err != nil {
			return err
		}
	}
	return nil
}

func formatHex(value []byte) string {
	encoded := strings.ToUpper(hex.EncodeToString(value))
	parts := make([]string, 0, len(value))
	for index := 0; index < len(encoded); index += 2 {
		parts = append(parts, encoded[index:index+2])
	}
	return strings.Join(parts, " ")
}

func setMagic(typeName, inputPath string, stdout io.Writer) error {
	target, ok := resolveTarget(typeName)
	if !ok {
		return fmt.Errorf("unknown magic-number type: %s", typeName)
	}

	inputPath, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve input path: %w", err)
	}
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("failed to inspect input file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("input is not a regular file: %s", inputPath)
	}

	sourceHash, err := hashReader(input)
	if err != nil {
		return fmt.Errorf("failed to hash input file: %w", err)
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to rewind input file: %w", err)
	}
	source, err := detectSignature(input)
	if err != nil {
		return fmt.Errorf("failed to inspect input magic number: %w", err)
	}
	if source != nil && source.Name == target.Name {
		return fmt.Errorf("file already has %s magic number: %s", target.Name, inputPath)
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to rewind input file: %w", err)
	}

	outputPath, err := nextOutputPath(inputPath, target.Name)
	if err != nil {
		return err
	}
	mode := "prepend"
	removed := []byte(nil)
	if source != nil {
		mode = "replace"
		removed = append(removed, source.Bytes...)
		if _, err := input.Seek(int64(len(source.Bytes)), io.SeekStart); err != nil {
			return fmt.Errorf("failed to skip source magic number: %w", err)
		}
	}

	temporary, err := os.CreateTemp(filepath.Dir(outputPath), ".xmagic-*")
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	published := false
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		if published && !committed {
			_ = os.Remove(outputPath)
		}
	}()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to set output permissions: %w", err)
	}
	if _, err := temporary.Write(target.Bytes); err != nil {
		return fmt.Errorf("failed to write target magic number: %w", err)
	}
	if _, err := io.Copy(temporary, input); err != nil {
		return fmt.Errorf("failed to copy input data: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("failed to sync output file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close output file: %w", err)
	}
	if err := os.Link(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("failed to publish output file: %w", err)
	}
	published = true

	outputHash, err := hashFile(outputPath)
	if err != nil {
		return fmt.Errorf("failed to hash output file: %w", err)
	}
	contentType, err := detectContentType(outputPath)
	if err != nil {
		return fmt.Errorf("failed to inspect output file: %w", err)
	}
	record := operation{
		Version:      1,
		Mode:         mode,
		SourcePath:   inputPath,
		OutputPath:   outputPath,
		TargetType:   target.Name,
		RemovedBytes: strings.ToUpper(hex.EncodeToString(removed)),
		AddedBytes:   strings.ToUpper(hex.EncodeToString(target.Bytes)),
		SourceSHA256: sourceHash,
		OutputSHA256: outputHash,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if source != nil {
		record.SourceType = source.Name
	}
	if err := saveOperation(record); err != nil {
		return fmt.Errorf("failed to save restoration state: %w", err)
	}
	committed = true

	if mode == "replace" {
		_, _ = fmt.Fprintf(stdout, "Replaced %s magic number with %s\n", source.Name, target.Name)
	} else {
		_, _ = fmt.Fprintf(stdout, "Prepended %s magic number\n", target.Name)
	}
	_, _ = fmt.Fprintf(stdout, "Created: %s\n", outputPath)
	_, _ = fmt.Fprintf(stdout, "Detected: %s\n", contentType)
	_, _ = fmt.Fprintf(stdout, "SHA-256: %s -> %s\n", sourceHash, outputHash)
	_, err = fmt.Fprintf(stdout, "Original unchanged: %s\n", inputPath)
	return err
}

func targetSignatures() []signature {
	var result []signature
	for _, item := range signatures {
		if !item.SourceOnly {
			result = append(result, item)
		}
	}
	return result
}

func resolveTarget(name string) (signature, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, item := range signatures {
		if item.SourceOnly {
			continue
		}
		if name == item.Name {
			return item, true
		}
		for _, alias := range item.Aliases {
			if name == alias {
				return item, true
			}
		}
	}
	return signature{}, false
}

func detectSignature(reader io.Reader) (*signature, error) {
	longest := 0
	for _, item := range signatures {
		if len(item.Bytes) > longest {
			longest = len(item.Bytes)
		}
	}
	prefix := make([]byte, longest)
	count, err := io.ReadFull(reader, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	prefix = prefix[:count]
	candidates := append([]signature(nil), signatures...)
	sort.SliceStable(candidates, func(i, j int) bool { return len(candidates[i].Bytes) > len(candidates[j].Bytes) })
	for i := range candidates {
		if bytes.HasPrefix(prefix, candidates[i].Bytes) {
			matched := candidates[i]
			return &matched, nil
		}
	}
	return nil, nil
}

func nextOutputPath(inputPath, target string) (string, error) {
	directory := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	if stem == "" {
		stem, extension = base, ""
	}
	for number := 1; ; number++ {
		suffix := "." + target
		if number > 1 {
			suffix += fmt.Sprintf(".%d", number)
		}
		candidate := filepath.Join(directory, stem+suffix+extension)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("failed to inspect output path: %w", err)
		}
	}
}

func hashReader(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return hashReader(file)
}

func detectContentType(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	prefix := make([]byte, 512)
	count, err := file.Read(prefix)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return http.DetectContentType(prefix[:count]), nil
}
