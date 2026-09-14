package docs

import "testing"

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
