package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// isPDF pozná PDF podle hlavičky „%PDF-“ (přípona souboru se nehodnotí).
func isPDF(b []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(b, "\xef\xbb\xbf\r\n\t "), []byte("%PDF-"))
}

// ExtractPDFText vytáhne text z PDF. Funguje jen u PDF s textovou vrstvou (ne u skenů).
func ExtractPDFText(b []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("PDF se nepodařilo přečíst (poškozený nebo nepodporovaný soubor)")
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("PDF se nepodařilo přečíst: %v", err)
	}
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		s, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(s)
		sb.WriteString("\n")
	}
	text = sb.String()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("v PDF není žádný text (je to nejspíš sken – potřebuje nejdřív rozpoznání textu, OCR)")
	}
	return text, nil
}

// TextFromBytes vrátí text souboru: z PDF vytáhne text, ostatní soubory bere jako UTF-8.
func TextFromBytes(b []byte) (string, error) {
	if isPDF(b) {
		return ExtractPDFText(b)
	}
	return string(b), nil
}

// readInputText načte soubor (nebo standardní vstup pro „-“) jako text; umí i PDF.
func readInputText(path string) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	return TextFromBytes(b)
}
