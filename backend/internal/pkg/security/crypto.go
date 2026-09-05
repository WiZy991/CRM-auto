package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// --- Случайные значения -----------------------------------------------------

// RandomToken возвращает криптостойкий токен в URL-безопасном base64.
// Используется для refresh-токенов и одноразовых ссылок.
func RandomToken(bytesLen int) (string, error) {
	if bytesLen < 16 {
		bytesLen = 32
	}
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("генерация случайного токена: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// NumericCode генерирует код подтверждения из цифр.
//
// Используется crypto/rand, а не math/rand: предсказуемый код
// подтверждения означает возможность подтвердить чужой телефон.
func NumericCode(digits int) (string, error) {
	if digits < 4 || digits > 10 {
		digits = 6
	}
	max := big.NewInt(1)
	for range digits {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("генерация кода подтверждения: %w", err)
	}
	return fmt.Sprintf("%0*d", digits, n), nil
}

// HashToken возвращает SHA-256 от токена.
//
// В базе хранится только хеш: refresh-токены не являются паролями, к ним не
// нужен argon2 (они и так случайны), но и в открытом виде их держать нельзя —
// дамп таблицы сессий давал бы прямой доступ к аккаунтам.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// --- Шифрование персональных данных -----------------------------------------

// Cipher шифрует поля повышенной чувствительности (паспортные данные,
// адреса) алгоритмом AES-256-GCM.
//
// Шифрование выполняется приложением, а не базой: даже полный дамп
// PostgreSQL или украденный бэкап не раскрывает эти поля без ключа,
// который лежит в переменных окружения, а не в базе.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher создаёт шифратор на 32-байтовом ключе.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("ключ шифрования должен быть длиной 32 байта, получено %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("инициализация AES: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("инициализация GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

var errCiphertextTooShort = errors.New("шифротекст короче, чем требуется")

// Encrypt возвращает nonce || ciphertext. Пустая строка шифруется в nil,
// чтобы в базе оставался NULL, а не бесполезные 28 байт.
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("генерация nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt расшифровывает значение, полученное из Encrypt.
func (c *Cipher) Decrypt(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", nil
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) < nonceSize+16 {
		return "", errCiphertextTooShort
	}
	nonce, ciphertext := payload[:nonceSize], payload[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Ошибка аутентификации означает либо неверный ключ, либо изменённые
		// данные. Подробности наружу не выдаются.
		return "", errors.New("не удалось расшифровать данные")
	}
	return string(plaintext), nil
}
