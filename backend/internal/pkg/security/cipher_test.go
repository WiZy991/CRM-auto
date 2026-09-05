package security

import "testing"

func TestCipherRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("dev-only-pii-key-32-bytes-long!"))
	if len(key) != 32 {
		t.Fatalf("длина ключа %d, нужно 32", len(key))
	}

	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	plain := "4500 123456 серия 99"
	enc, err := cipher.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(enc) == 0 {
		t.Fatal("шифротекст пуст")
	}
	if string(enc) == plain {
		t.Fatal("значение не зашифровано")
	}

	got, err := cipher.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Errorf("Decrypt = %q, ожидалось %q", got, plain)
	}
}

func TestCipherEmptyIsNil(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("dev-only-pii-key-32-bytes-long!"))
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt("")
	if err != nil || enc != nil {
		t.Fatalf("пустая строка должна давать nil, получено %v %v", enc, err)
	}
	got, err := cipher.Decrypt(nil)
	if err != nil || got != "" {
		t.Fatalf("nil расшифровывается в пустую строку, получено %q %v", got, err)
	}
}

func TestCipherRejectsWrongKeyLength(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatal("короткий ключ должен быть отклонён")
	}
}
