package docs

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestExtractMarkersAndAutoMap(t *testing.T) {
	text := `Договор № {{deal.number}} с «ФИО клиента» и VIN [VIN]`
	markers := ExtractMarkers(text)
	if len(markers) < 3 {
		t.Fatalf("ожидали маркеры, получили %#v", markers)
	}
	m := AutoMap(markers)
	if m["{{deal.number}}"] != "deal.number" {
		t.Fatalf("deal.number: %#v", m)
	}
	if m["«ФИО клиента»"] != "client.name" {
		t.Fatalf("client.name: %#v", m)
	}
	if m["[VIN]"] != "car.vin" {
		t.Fatalf("car.vin: %#v", m)
	}
}

func TestBuildReplacementsIncludesAllCatalogKeys(t *testing.T) {
	values := map[string]string{
		"client.name": "Иванов",
		"deal.amount": "1 000 ₽",
		"car.vin":     "XW8ZZZ",
	}
	repl := BuildReplacements(nil, values)
	if repl["{{client.name}}"] != "Иванов" {
		t.Fatalf("client.name: %q", repl["{{client.name}}"])
	}
	if repl["{{deal.amount}}"] != "1 000 ₽" {
		t.Fatalf("deal.amount: %q", repl["{{deal.amount}}"])
	}
	if repl["{{car.vin}}"] != "XW8ZZZ" {
		t.Fatalf("car.vin: %q", repl["{{car.vin}}"])
	}
	// Пустые поля → тире, но ключ всё равно в карте замен.
	if _, ok := repl["{{dealer.legal}}"]; !ok {
		t.Fatal("ожидали {{dealer.legal}} в заменах")
	}
}

func TestFillDocxReplacesSplitRuns(t *testing.T) {
	// Минимальный DOCX: zip с word/document.xml, плейсхолдер разбит на два <w:t>.
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>{{cli</w:t></w:r>
      <w:r><w:t>ent.name}}</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(document)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	filled, err := FillDocx(raw.Bytes(), nil, map[string]string{"client.name": "Петров Пётр"})
	if err != nil {
		t.Fatal(err)
	}
	text, err := ReadDocxText(filled)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(text), []byte("Петров Пётр")) {
		t.Fatalf("не подставилось ФИО, текст: %q", text)
	}
	if bytes.Contains([]byte(text), []byte("{{")) {
		t.Fatalf("остался плейсхолдер: %q", text)
	}
}
