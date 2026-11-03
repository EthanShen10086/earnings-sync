package acquire

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

// Hash identifies a document for a ticker.
func Hash(ticker string, body []byte) string {
	sum := sha256.Sum256(append([]byte(strings.ToUpper(ticker)+"\n"), body...))
	return hex.EncodeToString(sum[:])
}

// ReadFile turns an uploaded file into a document and best-effort text.
func ReadFile(filename string, body []byte) (model.Document, error) {
	if len(body) == 0 {
		return model.Document{}, fmt.Errorf("empty file")
	}
	doc := model.Document{
		Name:     path.Base(filename),
		Filename: filename,
		Body:     body,
		Source:   "file",
		Locator:  "file:" + path.Base(filename),
		MIME:     kind(filename, body),
	}
	text, err := ToText(doc)
	if err != nil {
		return model.Document{}, err
	}
	doc.Text = text
	return doc, nil
}

// FetchURL downloads a public page or file.
func FetchURL(ctx context.Context, client *http.Client, rawURL, userAgent string) (model.Document, error) {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return model.Document{}, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	req.Header.Set("Accept", "text/html,application/pdf,text/plain,text/csv,application/json,*/*")
	req.Header.Set("Accept-Encoding", "identity")
	res, err := client.Do(req)
	if err != nil {
		return model.Document{}, err
	}
	defer res.Body.Close()
	var body []byte
	if res.StatusCode == http.StatusForbidden && strings.Contains(rawURL, "sec.gov") {
		body, err = secGet(ctx, rawURL, userAgent)
		if err != nil {
			return model.Document{}, err
		}
	} else if res.StatusCode >= 300 {
		return model.Document{}, fmt.Errorf("GET %s: %s", rawURL, res.Status)
	} else {
		body, err = io.ReadAll(io.LimitReader(res.Body, 20<<20))
		if err != nil {
			return model.Document{}, err
		}
	}
	name := path.Base(rawURL)
	doc := model.Document{
		Name:     name,
		Filename: name,
		Body:     body,
		Source:   "url",
		Locator:  rawURL,
		MIME:     res.Header.Get("Content-Type"),
	}
	if doc.MIME == "" {
		doc.MIME = kind(name, body)
	}
	text, err := ToText(doc)
	if err != nil {
		return model.Document{}, err
	}
	doc.Text = text
	return doc, nil
}

// ToText extracts readable text from the document body.
func ToText(doc model.Document) (string, error) {
	mime := strings.ToLower(doc.MIME + " " + doc.Filename)
	switch {
	case strings.Contains(mime, "pdf") || strings.HasSuffix(strings.ToLower(doc.Filename), ".pdf"):
		return pdfText(doc.Body)
	case strings.Contains(mime, "spreadsheet") || strings.Contains(mime, "excel") ||
		strings.HasSuffix(strings.ToLower(doc.Filename), ".xlsx"):
		return xlsxText(doc.Body)
	default:
		return StatementText(string(doc.Body)), nil
	}
}

func pdfText(body []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("pdf: %w", err)
	}
	rd, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("pdf text: %w", err)
	}
	b, err := io.ReadAll(rd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func xlsxText(body []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("xlsx: %w", err)
	}
	defer f.Close()
	var b strings.Builder
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return "", err
		}
		b.WriteString("# " + name + "\n")
		for _, row := range rows {
			b.WriteString(strings.Join(row, ","))
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func kind(filename string, body []byte) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".xlsx"):
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	case strings.HasSuffix(lower, ".txt"):
		return "text/plain"
	default:
		return http.DetectContentType(body)
	}
}

// CSVRows reads a csv document. Non-csv text returns false.
func CSVRows(doc model.Document) ([][]string, bool) {
	mime := strings.ToLower(doc.MIME + " " + doc.Filename + " " + doc.Text)
	if !strings.Contains(mime, "csv") && !strings.Contains(doc.Text, "line_item") && !strings.Contains(doc.Text, "fiscal_year") {
		return nil, false
	}
	r := csv.NewReader(strings.NewReader(doc.Text))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, false
	}
	return rows, true
}
