package social

import "testing"

func TestBuildCaption(t *testing.T) {
	got := BuildCaption("Geely", "Coolray", 2024, "2 150 000 ₽", "https://example.test/catalog/1")
	if got != "Geely Coolray 2024\n2 150 000 ₽\nhttps://example.test/catalog/1" {
		t.Fatalf("неожиданный текст поста:\n%s", got)
	}
}

func TestSecretMask(t *testing.T) {
	if SecretMask("") != "" {
		t.Fatal("пустой секрет должен оставаться пустым")
	}
	if SecretMask("abcd") != "••••" {
		t.Fatalf("короткий секрет: %q", SecretMask("abcd"))
	}
	if got := SecretMask("1234567890a1b2"); got != "…a1b2" {
		t.Fatalf("маска: %q", got)
	}
}
