package docs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

var (
	reParagraph = regexp.MustCompile(`(?s)<w:p[\s>](?:.*?)</w:p>`)
	reTextRun   = regexp.MustCompile(`(?s)<w:t([^>]*)>(.*?)</w:t>`)
)

// BuildReplacements собирает все подстановки: каждый ключ Payload как {{key}},
// плюс ручной field_map и известные русские маркеры.
func BuildReplacements(fieldMap map[string]string, values map[string]string) map[string]string {
	out := make(map[string]string, len(values)*2+len(fieldMap))

	display := func(key string) string {
		val := strings.TrimSpace(values[key])
		if val == "" || val == missing {
			return "—"
		}
		return val
	}

	for key := range fieldTitles {
		val := escapeXML(display(key))
		out["{{"+key+"}}"] = val
		out["{{ "+key+" }}"] = val
	}

	// Синонимы в «ёлочках» / скобках — только явные маркеры, не живой текст договора.
	for key, synonyms := range synonymIndex {
		val := escapeXML(display(key))
		for _, syn := range synonyms {
			out["«"+syn+"»"] = val
			out["["+syn+"]"] = val
			if title := titleCaseRu(syn); title != syn {
				out["«"+title+"»"] = val
				out["["+title+"]"] = val
			}
		}
	}

	for marker, key := range fieldMap {
		if key == "" {
			continue
		}
		out[marker] = escapeXML(display(key))
		if strings.HasPrefix(marker, "{{") && strings.HasSuffix(marker, "}}") {
			inner := strings.TrimSpace(marker[2 : len(marker)-2])
			out["{{"+inner+"}}"] = out[marker]
		}
	}
	return out
}

// FillDocx подставляет значения сделки во все {{поля}} разом (и в field_map).
func FillDocx(docx []byte, fieldMap map[string]string, values map[string]string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("docx: %w", err)
	}

	replacements := BuildReplacements(fieldMap, values)
	// Длинные маркеры первыми — чтобы «ФИО клиента» не перебивалось «ФИО».
	markers := make([]string, 0, len(replacements))
	for marker := range replacements {
		markers = append(markers, marker)
	}
	sort.Slice(markers, func(i, j int) bool {
		return len(markers[i]) > len(markers[j])
	})

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
			text = fillInParagraphs(text, markers, replacements)
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

// ReadDocxText извлекает плоский текст (для парсинга маркеров).
func ReadDocxText(docx []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return "", fmt.Errorf("docx: повреждённый файл (%w)", err)
	}
	var b strings.Builder
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "word/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		payload, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return "", err
		}
		// Сначала склеиваем параграфы — Word дробит {{client.name}} по runs.
		joined := fillInParagraphs(string(payload), nil, nil)
		b.WriteString(stripXML(joined))
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("docx: нет word/*.xml")
	}
	return b.String(), nil
}

// fillInParagraphs склеивает текст внутри каждого <w:p> в один <w:t>,
// чтобы плейсхолдеры, разбитые Word, снова стали целыми, и подставляет значения.
func fillInParagraphs(xml string, markers []string, replacements map[string]string) string {
	return reParagraph.ReplaceAllStringFunc(xml, func(para string) string {
		runs := reTextRun.FindAllStringSubmatchIndex(para, -1)
		if len(runs) == 0 {
			return para
		}

		var plain strings.Builder
		type span struct{ start, end int } // позиции в plain
		spans := make([]span, 0, len(runs))
		for _, loc := range runs {
			// loc: full, attr, text — groups
			textStart, textEnd := loc[4], loc[5]
			content := para[textStart:textEnd]
			decoded := xmlUnescape(content)
			start := plain.Len()
			plain.WriteString(decoded)
			spans = append(spans, span{start, plain.Len()})
		}

		text := plain.String()
		if len(markers) > 0 {
			for _, marker := range markers {
				if !strings.Contains(text, marker) {
					continue
				}
				text = strings.ReplaceAll(text, marker, replacements[marker])
			}
		}

		// Если текст не менялся и маркеров не было — всё равно склеим mustache-фрагменты.
		if len(markers) == 0 {
			text = coalesceMustacheInPlain(text)
		}

		if text == plain.String() && len(markers) == 0 {
			// Только склейка mustache для извлечения: перепишем первый run целиком.
			if !strings.Contains(text, "{{") {
				return para
			}
		}

		// Пишем весь текст в первый <w:t>, остальные очищаем.
		var out strings.Builder
		cursor := 0
		first := true
		for i, loc := range runs {
			out.WriteString(para[cursor:loc[0]])
			attrs := para[loc[2]:loc[3]]
			if first {
				// preserve space если есть пробелы по краям
				if !strings.Contains(attrs, "xml:space") && (strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ")) {
					attrs += ` xml:space="preserve"`
				}
				out.WriteString("<w:t")
				out.WriteString(attrs)
				out.WriteString(">")
				out.WriteString(escapeXML(text))
				out.WriteString("</w:t>")
				first = false
			} else {
				out.WriteString("<w:t")
				out.WriteString(attrs)
				out.WriteString("></w:t>")
			}
			_ = i
			cursor = loc[1]
		}
		out.WriteString(para[cursor:])
		return out.String()
	})
}

func coalesceMustacheInPlain(s string) string {
	re := regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)
	return re.ReplaceAllStringFunc(s, func(m string) string {
		parts := re.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return "{{" + parts[1] + "}}"
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

func xmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		`&amp;`, "&",
		`&lt;`, "<",
		`&gt;`, ">",
		`&quot;`, `"`,
		`&apos;`, "'",
	)
	return replacer.Replace(s)
}

func titleCaseRu(s string) string {
	runes := []rune(strings.ToLower(s))
	if len(runes) == 0 {
		return s
	}
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}
