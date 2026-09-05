package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestSlugOK(t *testing.T) {
	ok := []string{"ab", "vostok-auto", "d-abc123def456", "tokyo7"}
	for _, slug := range ok {
		if !SlugOK(slug) {
			t.Errorf("SlugOK(%q) = false, ожидалось true", slug)
		}
	}

	bad := []string{"", "A", "-start", "Пробел", "UPPER", "has_underscore", "a"}
	for _, slug := range bad {
		if SlugOK(slug) {
			t.Errorf("SlugOK(%q) = true, ожидалось false", slug)
		}
	}
}

func TestDefaultDealerSlug(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	got := defaultDealerSlug(id)
	if !SlugOK(got) {
		t.Errorf("defaultDealerSlug не проходит SlugOK: %q", got)
	}
	if got[:2] != "d-" {
		t.Errorf("ожидался префикс d-, получено %q", got)
	}
}
