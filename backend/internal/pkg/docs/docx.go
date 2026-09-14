package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	reXMLTagsBetween = regexp.MustCompile(`\{\{[^}]*\}?|\[[^\]]*\]?|«[^»]*`)
	reSplitMustache  = regexp.MustCompile(`\{\{(?:<[^>]+>)*([^}<]+)(?:<[^>]+>)*\}\}`)
)

// ReadDocxText извлекает текст из document.xml (для парсинга маркеров).
func ReadDocxText(docx []byte) (string, error) {
	xmls, err := readDocxXMLs(docx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, raw := range xmls {
		b.WriteString(stripXML(raw))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// FillDocx подставляет значения по field_map: marker → payload key → value.
func FillDocx(docx []byte, fieldMap map[string]string, values map[string]string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("docx: %w", err)
	}

	replacements := make(map[string]string, len(fieldMap))
	for marker, key := range fieldMap {
		val := values[key]
		if val == "" || val == missing {
			val = "—"
		}
		replacements[marker] = escapeXML(val)
		// Также подставляем «чистый» mustache, если маркер был {{key}}.
		if strings.HasPrefix(marker, "{{") && strings.HasSuffix(marker, "}}") {
			inner := strings.TrimSpace(marker[2 : len(marker)-2])
			replacements["{{"+inner+"}}"] = escapeXML(val)
		}
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		payload, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}

		name := file.Name
		if strings.HasPrefix(name, "word/") && strings.HasSuffix(name, ".xml") {
			text := string(payload)
			text = coalesceSplitPlaceholders(text)
			for marker, val := range replacements {
				text = strings.ReplaceAll(text, marker, val)
				text = strings.ReplaceAll(text, escapeXML(marker), val)
			}
			payload = []byte(text)
		}

		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(file.Mode())
		out, err := w.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := out.Write(payload); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readDocxXMLs(docx []byte) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("docx: повреждённый файл (%w)", err)
	}
	var out []string
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "word/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		payload, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, coalesceSplitPlaceholders(string(payload)))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("docx: нет word/*.xml")
	}
	return out, nil
}

// coalesceSplitPlaceholders склеивает плейсхолдеры, разбитые Word по <w:t>.
func coalesceSplitPlaceholders(xml string) string {
	return reSplitMustache.ReplaceAllStringFunc(xml, func(m string) string {
		inner := reSplitMustache.FindStringSubmatch(m)
		if len(inner) < 2 {
			return m
		}
		return "{{" + strings.TrimSpace(stripXML(inner[1])) + "}}"
	})
}

func stripXML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func escapeXML(s string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return replacer.Replace(s)
}

// Suppress unused lint for helper kept for future quote-marker coalesce.
var _ = reXMLTagsBetween
