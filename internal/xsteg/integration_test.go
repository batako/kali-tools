package xsteg

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRealSteghideAndStegseekExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("external backend integration test")
	}
	for _, command := range []string{"steghide", "stegseek"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skip(command + " is not installed")
		}
	}

	directory := t.TempDir()
	cover := filepath.Join(directory, "cover.bmp")
	stego := filepath.Join(directory, "stego.bmp")
	secret := filepath.Join(directory, "secret.txt")
	wordlist := filepath.Join(directory, "passwords.txt")
	if err := os.WriteFile(cover, testBMP(100, 100), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("xsteg integration secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wordlist, []byte("wrong\nhunter2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("steghide", "embed", "-cf", cover, "-ef", secret, "-sf", stego, "-p", "hunter2", "-f", "-q")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("steghide embed: %v: %s", err, output)
	}

	if err := Run([]string{"xsteg", "scan", stego}, bytes.NewReader(nil), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"xsteg", "extract", stego, "--wordlist", wordlist}, bytes.NewReader(nil), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	report, err := loadReport(filepath.Join(stego+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range report.Findings {
		if finding.Backend != "stegseek" || finding.Kind != "extracted" {
			continue
		}
		content, err := os.ReadFile(finding.Path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "xsteg integration secret\n" || finding.Password != "hunter2" || finding.OriginalName != "secret.txt" || filepath.Base(finding.Path) != "secret.txt" {
			t.Fatalf("content = %q, password = %q", content, finding.Password)
		}
		return
	}
	t.Fatalf("stegseek finding not found: %+v", report.Findings)
}

func TestRealManualSteghideExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("external backend integration test")
	}
	if _, err := exec.LookPath("steghide"); err != nil {
		t.Skip("steghide is not installed")
	}
	directory := t.TempDir()
	cover := filepath.Join(directory, "cover.bmp")
	stego := filepath.Join(directory, "stego.bmp")
	secret := filepath.Join(directory, "creds.txt")
	if err := os.WriteFile(cover, testBMP(100, 100), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("manual extraction secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("steghide", "embed", "-cf", cover, "-ef", secret, "-sf", stego, "-p", "AllmightForEver!!!", "-f", "-q")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("steghide embed: %v: %s", err, output)
	}
	if err := Run([]string{"xsteg", "scan", stego}, bytes.NewReader(nil), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	scanReport, err := loadReport(filepath.Join(stego+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !scanReport.ScanCompleted || !requiresPassphraseAnalysis(scanReport) {
		t.Fatalf("scan did not detect protected payload: %+v", scanReport.Findings)
	}
	if err := Run([]string{"xsteg", "extract", stego, "--manual"}, bytes.NewBufferString("AllmightForEver!!!\n"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	report, err := loadReport(filepath.Join(stego+".xsteg", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var finding *Finding
	for index := range report.Findings {
		if report.Findings[index].Kind == "extracted" {
			finding = &report.Findings[index]
		}
	}
	if finding == nil {
		t.Fatalf("findings = %+v, warnings = %+v", report.Findings, report.Warnings)
	}
	content, err := os.ReadFile(finding.Path)
	if err != nil {
		t.Fatal(err)
	}
	if finding.OriginalName != "creds.txt" || filepath.Base(finding.Path) != "creds.txt" || string(content) != "manual extraction secret\n" {
		t.Fatalf("finding = %+v, content = %q", finding, content)
	}
}

func testBMP(width, height int) []byte {
	rowSize := (width*3 + 3) &^ 3
	pixelSize := rowSize * height
	data := make([]byte, 54+pixelSize)
	copy(data[0:2], "BM")
	binary.LittleEndian.PutUint32(data[2:6], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[10:14], 54)
	binary.LittleEndian.PutUint32(data[14:18], 40)
	binary.LittleEndian.PutUint32(data[18:22], uint32(width))
	binary.LittleEndian.PutUint32(data[22:26], uint32(height))
	binary.LittleEndian.PutUint16(data[26:28], 1)
	binary.LittleEndian.PutUint16(data[28:30], 24)
	binary.LittleEndian.PutUint32(data[34:38], uint32(pixelSize))
	for index := 54; index < len(data); index += 3 {
		data[index] = byte(index)
		if index+1 < len(data) {
			data[index+1] = byte(index >> 2)
		}
		if index+2 < len(data) {
			data[index+2] = byte(index >> 4)
		}
	}
	return data
}
