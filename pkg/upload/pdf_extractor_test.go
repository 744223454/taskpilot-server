package upload

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParsePDFInfo(t *testing.T) {
	pages, encrypted, err := parsePDFInfo("Pages: 12\nEncrypted: no\n")
	if err != nil || pages != 12 || encrypted {
		t.Fatalf("parsePDFInfo() = %d, %v, %v", pages, encrypted, err)
	}
	pages, encrypted, err = parsePDFInfo("Pages: 1\nEncrypted: yes (print:yes copy:no)\n")
	if err != nil || pages != 1 || !encrypted {
		t.Fatalf("encrypted parsePDFInfo() = %d, %v, %v", pages, encrypted, err)
	}
}

func TestPopplerExtractorMapsLimitsAndUsableText(t *testing.T) {
	directory := t.TempDir()
	pdfInfo := filepath.Join(directory, "pdfinfo")
	pdfToText := filepath.Join(directory, "pdftotext")
	writeExecutable(t, pdfInfo, "#!/bin/sh\nprintf 'Pages: 2\\nEncrypted: no\\n'\n")
	writeExecutable(t, pdfToText, "#!/bin/sh\nprintf 'Useful PDF body text for TaskPilot.'\n")

	extractor := NewPopplerExtractor(50, 50000, 20, 1, 5*time.Second, time.Second)
	extractor.pdfInfoCommand = pdfInfo
	extractor.pdfToTextCommand = pdfToText
	result, err := extractor.Extract(context.Background(), filepath.Join(directory, "input.pdf"))
	if err != nil || result.PageCount != 2 || result.Text != "Useful PDF body text for TaskPilot." {
		t.Fatalf("Extract() = %#v, %v", result, err)
	}

	writeExecutable(t, pdfInfo, "#!/bin/sh\nprintf 'Pages: 51\\nEncrypted: no\\n'\n")
	if _, err := extractor.Extract(context.Background(), "input.pdf"); !errors.Is(err, ErrPDFTooManyPages) {
		t.Fatalf("page-limit Extract() error = %v", err)
	}

	writeExecutable(t, pdfInfo, "#!/bin/sh\nprintf 'Pages: 1\\nEncrypted: no\\n'\n")
	writeExecutable(t, pdfToText, "#!/bin/sh\nprintf '   \\n\\t'\n")
	if _, err := extractor.Extract(context.Background(), "input.pdf"); !errors.Is(err, ErrPDFNoUsableText) {
		t.Fatalf("empty-text Extract() error = %v", err)
	}
}

func TestPopplerExtractorBusy(t *testing.T) {
	extractor := NewPopplerExtractor(50, 50000, 20, 1, time.Second, 10*time.Millisecond)
	extractor.slots <- struct{}{}
	defer func() { <-extractor.slots }()
	if _, err := extractor.Extract(context.Background(), "input.pdf"); !errors.Is(err, ErrPDFExtractorBusy) {
		t.Fatalf("Extract() error = %v, want ErrPDFExtractorBusy", err)
	}
}

func TestPopplerExtractorRealPDF(t *testing.T) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo is not installed")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed")
	}
	path := filepath.Join(t.TempDir(), "fixture.pdf")
	if err := os.WriteFile(path, minimalTextPDF("TaskPilot PDF extraction fixture text."), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	extractor := NewPopplerExtractor(50, 50000, 20, 1, 15*time.Second, time.Second)
	result, err := extractor.Extract(context.Background(), path)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.PageCount != 1 || !strings.Contains(result.Text, "TaskPilot PDF extraction fixture text") {
		t.Fatalf("Extract() result = %#v", result)
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}

func minimalTextPDF(text string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		"<< /Length " + stringLength("BT /F1 12 Tf 72 720 Td ("+text+") Tj ET") + " >>\nstream\nBT /F1 12 Tf 72 720 Td (" + text + ") Tj ET\nendstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var builder strings.Builder
	builder.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = builder.Len()
		builder.WriteString(strings.TrimSpace(strings.Join([]string{strconv.Itoa(index + 1), "0 obj\n", object, "\nendobj\n"}, " ")))
		builder.WriteByte('\n')
	}
	xref := builder.Len()
	builder.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for index := 1; index <= len(objects); index++ {
		builder.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[index]))
	}
	builder.WriteString(fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref))
	return []byte(builder.String())
}

func stringLength(value string) string {
	return strconv.Itoa(len(value) + 1)
}
