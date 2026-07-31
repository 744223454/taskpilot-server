package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrPDFInvalid        = errors.New("invalid PDF")
	ErrPDFEncrypted      = errors.New("encrypted PDF")
	ErrPDFTooManyPages   = errors.New("PDF page limit exceeded")
	ErrPDFTextTooLarge   = errors.New("PDF text limit exceeded")
	ErrPDFNoUsableText   = errors.New("PDF has no usable text")
	ErrPDFExtractTimeout = errors.New("PDF extraction timed out")
	ErrPDFExtractorBusy  = errors.New("PDF extractor is busy")
)

type PDFResult struct {
	Text      string
	PageCount int32
}

type PDFExtractor interface {
	Extract(context.Context, string) (PDFResult, error)
}

type PopplerExtractor struct {
	maxPages          int
	maxTextChars      int
	minEffectiveChars int
	timeout           time.Duration
	slotWait          time.Duration
	slots             chan struct{}
	pdfInfoCommand    string
	pdfToTextCommand  string
}

func NewPopplerExtractor(maxPages, maxTextChars, minEffectiveChars, concurrency int, timeout, slotWait time.Duration) *PopplerExtractor {
	if maxPages <= 0 {
		maxPages = 50
	}
	if maxTextChars <= 0 {
		maxTextChars = 50000
	}
	if minEffectiveChars <= 0 {
		minEffectiveChars = 20
	}
	if concurrency <= 0 {
		concurrency = 2
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if slotWait <= 0 {
		slotWait = 3 * time.Second
	}
	return &PopplerExtractor{
		maxPages: maxPages, maxTextChars: maxTextChars, minEffectiveChars: minEffectiveChars,
		timeout: timeout, slotWait: slotWait, slots: make(chan struct{}, concurrency),
		pdfInfoCommand: "pdfinfo", pdfToTextCommand: "pdftotext",
	}
}

func (e *PopplerExtractor) Extract(ctx context.Context, path string) (PDFResult, error) {
	if err := e.acquire(ctx); err != nil {
		return PDFResult{}, err
	}
	defer func() { <-e.slots }()

	extractContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	infoOutput, err := exec.CommandContext(extractContext, e.pdfInfoCommand, path).CombinedOutput()
	if err != nil {
		if errors.Is(extractContext.Err(), context.DeadlineExceeded) {
			return PDFResult{}, ErrPDFExtractTimeout
		}
		var executableError *exec.Error
		if errors.As(err, &executableError) {
			return PDFResult{}, fmt.Errorf("run pdfinfo: %w", err)
		}
		return PDFResult{}, fmt.Errorf("%w: pdfinfo failed: %s", ErrPDFInvalid, limitedMessage(infoOutput))
	}
	pageCount, encrypted, err := parsePDFInfo(string(infoOutput))
	if err != nil {
		return PDFResult{}, fmt.Errorf("%w: %v", ErrPDFInvalid, err)
	}
	if encrypted {
		return PDFResult{}, ErrPDFEncrypted
	}
	if pageCount > e.maxPages {
		return PDFResult{}, ErrPDFTooManyPages
	}

	var stdout limitedBuffer
	stdout.limit = maxTextBytes(e.maxTextChars)
	var stderr limitedBuffer
	stderr.limit = 4096
	command := exec.CommandContext(extractContext, e.pdfToTextCommand, "-enc", "UTF-8", "-nopgbrk", path, "-")
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if errors.Is(stdout.err, errLimitExceeded) {
		return PDFResult{}, ErrPDFTextTooLarge
	}
	if err != nil {
		if errors.Is(extractContext.Err(), context.DeadlineExceeded) {
			return PDFResult{}, ErrPDFExtractTimeout
		}
		var executableError *exec.Error
		if errors.As(err, &executableError) {
			return PDFResult{}, fmt.Errorf("run pdftotext: %w", err)
		}
		return PDFResult{}, fmt.Errorf("%w: pdftotext failed: %s", ErrPDFInvalid, limitedMessage(stderr.Bytes()))
	}
	text := strings.TrimSpace(stdout.String())
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > e.maxTextChars {
		return PDFResult{}, ErrPDFTextTooLarge
	}
	if effectiveCharacters(text) < e.minEffectiveChars {
		return PDFResult{}, ErrPDFNoUsableText
	}
	return PDFResult{Text: text, PageCount: int32(pageCount)}, nil
}

func (e *PopplerExtractor) acquire(ctx context.Context) error {
	timer := time.NewTimer(e.slotWait)
	defer timer.Stop()
	select {
	case e.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrPDFExtractorBusy
	}
}

func parsePDFInfo(output string) (int, bool, error) {
	pages := 0
	encrypted := false
	for _, line := range strings.Split(output, "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(name)) {
		case "pages":
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 1 {
				return 0, false, errors.New("invalid page count")
			}
			pages = parsed
		case "encrypted":
			encrypted = strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "yes")
		}
	}
	if pages == 0 {
		return 0, false, errors.New("page count is missing")
	}
	return pages, encrypted, nil
}

func effectiveCharacters(text string) int {
	count := 0
	for _, character := range text {
		if !unicode.IsSpace(character) && !unicode.IsControl(character) {
			count++
		}
	}
	return count
}

func maxTextBytes(maxChars int) int {
	return maxChars*utf8.UTFMax + 1
}

var errLimitExceeded = errors.New("buffer limit exceeded")

type limitedBuffer struct {
	bytes.Buffer
	limit int
	err   error
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 || len(value) > remaining {
		if remaining > 0 {
			_, _ = b.Buffer.Write(value[:remaining])
		}
		b.err = errLimitExceeded
		return len(value), b.err
	}
	return b.Buffer.Write(value)
}

func limitedMessage(value []byte) string {
	message := strings.TrimSpace(string(value))
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
