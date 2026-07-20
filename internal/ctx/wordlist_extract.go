package ctx

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const wordlistExtractUsageText = `usage: ctx wordlist extract [options]

Verify and extract wordlists trusted by this ctx release.

options:
  -y, --yes       skip confirmation
  --force          replace an existing extracted file
  --remove-source  remove the trusted archive after successful extraction
  -h, --help       show this help`

type wordlistExtractSpec struct {
	ID                 string
	RelativeSourcePath string
	RelativeOutputPath string
	Format             string
	Kind               string
	SourceSHA256       string
	OutputSize         int64
}

var wordlistExtractSpecs = []wordlistExtractSpec{
	{
		ID:                 "rockyou",
		RelativeSourcePath: "rockyou.txt.gz",
		RelativeOutputPath: "rockyou.txt",
		Format:             "gzip",
		Kind:               WordlistKindPassword,
		SourceSHA256:       "d87371fe6ce95e3b8d68f537d1614232fdd47a928693dc35cfea5b89b8f713a7",
		OutputSize:         139921507,
	},
}

type wordlistExtractOptions struct {
	Yes          bool
	Force        bool
	RemoveSource bool
	Internal     bool
}

var reexecWordlistExtractWithSudoFunc = reexecWordlistExtractWithSudo

func runWordlistExtract(args []string, output outputOptions, stdin io.Reader, stdout io.Writer) error {
	if output.Format != apiFormatShell || output.FormatVersion != "" {
		return errors.New("ctx wordlist extract only supports shell output")
	}
	options, help, err := parseWordlistExtractOptions(args)
	if err != nil {
		return err
	}
	if help {
		_, err = fmt.Fprintln(stdout, wordlistExtractUsageText)
		return err
	}
	root := DiscoverWordlistsRoot()
	if root == "" {
		return errors.New("wordlists directory not found; install the wordlists package")
	}
	scanner := bufio.NewScanner(stdin)
	processed := 0
	for _, spec := range wordlistExtractSpecs {
		didExtract, err := extractTrustedWordlist(root, spec, options, scanner, stdout)
		if err != nil {
			if !options.Internal && errors.Is(err, os.ErrPermission) {
				if _, writeErr := fmt.Fprintln(stdout, "Need administrator privileges to extract wordlists."); writeErr != nil {
					return writeErr
				}
				if _, writeErr := fmt.Fprintln(stdout, "Re-running wordlist extract with sudo..."); writeErr != nil {
					return writeErr
				}
				return reexecWordlistExtractWithSudoFunc(options, stdout)
			}
			return err
		}
		if didExtract {
			processed++
		}
	}
	if processed == 0 {
		_, err = fmt.Fprintln(stdout, "No wordlists were extracted.")
		return err
	}
	return nil
}

func parseWordlistExtractOptions(args []string) (wordlistExtractOptions, bool, error) {
	var options wordlistExtractOptions
	for _, arg := range args {
		switch arg {
		case "-y", "--yes":
			options.Yes = true
		case "--force":
			options.Force = true
		case "--remove-source":
			options.RemoveSource = true
		case "--internal":
			options.Internal = true
		case "-h", "--help":
			return options, true, nil
		default:
			return options, false, fmt.Errorf("unknown wordlist extract option: %s", arg)
		}
	}
	return options, false, nil
}

func extractTrustedWordlist(root string, spec wordlistExtractSpec, options wordlistExtractOptions, scanner *bufio.Scanner, stdout io.Writer) (bool, error) {
	source := filepath.Join(root, filepath.FromSlash(spec.RelativeSourcePath))
	destination := filepath.Join(root, filepath.FromSlash(spec.RelativeOutputPath))
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		if os.IsNotExist(err) {
			if destinationInfo, destinationErr := os.Lstat(destination); destinationErr == nil && destinationInfo.Mode().IsRegular() {
				_, writeErr := fmt.Fprintf(stdout, "%s is already extracted: %s\n", spec.ID, destination)
				return false, writeErr
			}
			return false, fmt.Errorf("trusted wordlist source not found: %s", source)
		}
		return false, fmt.Errorf("failed to inspect %s: %w", source, err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return false, fmt.Errorf("trusted wordlist source must be a regular file: %s", source)
	}
	actualHash, err := sha256File(source)
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(actualHash, spec.SourceSHA256) {
		return false, fmt.Errorf("%s is not a trusted version: expected %s, got %s", source, spec.SourceSHA256, actualHash)
	}
	destinationInfo, destinationErr := os.Lstat(destination)
	if destinationErr == nil {
		if destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.Mode().IsRegular() {
			return false, fmt.Errorf("extraction destination must be a regular file: %s", destination)
		}
		if !options.Force {
			if options.RemoveSource {
				return false, errors.New("--remove-source cannot be used when the output already exists without --force")
			}
			_, writeErr := fmt.Fprintf(stdout, "%s is already extracted: %s\n", spec.ID, destination)
			return false, writeErr
		}
	} else if !os.IsNotExist(destinationErr) {
		return false, fmt.Errorf("failed to inspect %s: %w", destination, destinationErr)
	}
	if !options.Internal {
		if err := syscall.Access(filepath.Dir(destination), 2); err != nil {
			return false, fmt.Errorf("wordlist directory is not writable: %w", os.ErrPermission)
		}
	}
	if _, err := fmt.Fprintf(stdout, "%s\n  source: %s\n  SHA-256: verified\n  output: %s\n", spec.ID, source, destination); err != nil {
		return false, err
	}
	if !options.Yes {
		if _, err := fmt.Fprint(stdout, "Extract? [y/N] "); err != nil {
			return false, err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return false, fmt.Errorf("failed to read confirmation: %w", err)
			}
			_, err := fmt.Fprintln(stdout, "cancelled")
			return false, err
		}
		answer := strings.TrimSpace(scanner.Text())
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			_, err := fmt.Fprintln(stdout, "cancelled")
			return false, err
		}
	}
	if err := extractTrustedGzip(source, destination, spec, options.Force); err != nil {
		return false, err
	}
	if options.RemoveSource {
		currentHash, err := sha256File(source)
		if err != nil {
			return false, err
		}
		if !strings.EqualFold(currentHash, spec.SourceSHA256) {
			return false, errors.New("trusted source changed during extraction; refusing to remove it")
		}
		if err := os.Remove(source); err != nil {
			return false, fmt.Errorf("failed to remove source %s: %w", source, err)
		}
	}
	_, err = fmt.Fprintf(stdout, "Extracted %s (%d bytes)\n", destination, spec.OutputSize)
	return err == nil, err
}

func extractTrustedGzip(source, destination string, spec wordlistExtractSpec, force bool) error {
	if spec.Format != "gzip" {
		return fmt.Errorf("unsupported trusted wordlist format: %s", spec.Format)
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", source, err)
	}
	defer sourceFile.Close()
	gzipReader, err := gzip.NewReader(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read gzip %s: %w", source, err)
	}
	defer gzipReader.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".ctx-*")
	if err != nil {
		return fmt.Errorf("failed to create extraction file: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		_ = temporary.Close()
		if !keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("failed to set extraction permissions: %w", err)
	}
	written, err := io.Copy(temporary, io.LimitReader(gzipReader, spec.OutputSize+1))
	if err != nil {
		return fmt.Errorf("failed to extract %s: %w", source, err)
	}
	if written != spec.OutputSize {
		return fmt.Errorf("unexpected extracted size for %s: expected %d, got %d", source, spec.OutputSize, written)
	}
	probe := make([]byte, 1)
	if count, readErr := gzipReader.Read(probe); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return fmt.Errorf("unexpected data after trusted output size for %s", source)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("failed to sync extracted wordlist: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close extracted wordlist: %w", err)
	}
	if !force {
		if _, err := os.Lstat(destination); err == nil {
			return fmt.Errorf("extraction destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect extraction destination: %w", err)
		}
	}
	currentSourceHash, err := sha256File(source)
	if err != nil {
		return err
	}
	if !strings.EqualFold(currentSourceHash, spec.SourceSHA256) {
		return errors.New("trusted source changed during extraction; refusing to install the output")
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("failed to install extracted wordlist: %w", err)
	}
	keepTemporary = true
	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func reexecWordlistExtractWithSudo(options wordlistExtractOptions, stdout io.Writer) error {
	executable, err := executableFunc()
	if err != nil {
		return fmt.Errorf("failed to locate ctx executable: %w", err)
	}
	args := wordlistExtractSudoArgs(executable, options)
	cmd := execCommandFunc("sudo", args...)
	if workingDirectory, err := os.Getwd(); err == nil {
		cmd.Dir = workingDirectory
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo wordlist extract failed: %w", err)
	}
	return nil
}

func wordlistExtractSudoArgs(executable string, options wordlistExtractOptions) []string {
	args := []string{"env", "CTX_HOME=" + dataRoot(), executable, "wordlist", "extract", "--internal"}
	if options.Yes {
		args = append(args, "--yes")
	}
	if options.Force {
		args = append(args, "--force")
	}
	if options.RemoveSource {
		args = append(args, "--remove-source")
	}
	return args
}
