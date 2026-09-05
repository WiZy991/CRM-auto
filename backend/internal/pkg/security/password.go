// Package security собирает криптографические примитивы приложения:
// хеширование паролей, генерацию и проверку токенов, шифрование
// персональных данных.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Hasher хеширует и проверяет пароли алгоритмом argon2id.
//
// Выбран argon2id, а не bcrypt: bcrypt ограничен 72 байтами пароля и не
// сопротивляется атакам на GPU так же хорошо. Параметры (память, число
// проходов, параллелизм) хранятся внутри самой хеш-строки, поэтому их можно
// усилить в конфиге, не ломая уже сохранённые пароли.
type Hasher struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
	saltLen     uint32
	keyLen      uint32
}

// NewHasher создаёт хешер. Значения по умолчанию (64 МиБ, 3 прохода)
// соответствуют текущим рекомендациям OWASP для argon2id.
func NewHasher(memoryKiB, iterations uint32, parallelism uint8) *Hasher {
	if memoryKiB < 19*1024 {
		memoryKiB = 19 * 1024
	}
	if iterations < 2 {
		iterations = 2
	}
	if parallelism == 0 {
		parallelism = 1
	}
	return &Hasher{
		memoryKiB:   memoryKiB,
		iterations:  iterations,
		parallelism: parallelism,
		saltLen:     16,
		keyLen:      32,
	}
}

// Hash возвращает строку формата
// $argon2id$v=19$m=65536,t=3,p=2$<salt-b64>$<hash-b64>
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("генерация соли: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, h.iterations, h.memoryKiB, h.parallelism, h.keyLen)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.memoryKiB, h.iterations, h.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

var errBadHashFormat = errors.New("некорректный формат хеша пароля")

// Verify проверяет пароль. Сравнение выполняется за постоянное время, чтобы
// не давать атакующему информации через время ответа.
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, errBadHashFormat
	}
	if version != argon2.Version {
		return false, fmt.Errorf("несовместимая версия argon2: %d", version)
	}

	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, errBadHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHashFormat
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHashFormat
	}

	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// NeedsRehash сообщает, что хеш создан более слабыми параметрами, чем
// текущие. Пароль перехешируется при следующем успешном входе.
func (h *Hasher) NeedsRehash(encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	return memory < h.memoryKiB || iterations < h.iterations
}

// --- Политика паролей -------------------------------------------------------

// Требования к паролю намеренно умеренные: длина важнее «обязательного
// специального символа», который люди обходят предсказуемо (Password1!).
const (
	MinPasswordLength = 10
	MaxPasswordLength = 128
)

// weakPasswords — короткий офлайн-список самых частых паролей и подстрок,
// связанных с самой платформой. Полноценная проверка по базе утечек
// подключается отдельным адаптером в CheckBreached.
var weakPasswords = map[string]bool{
	"password":    true,
	"password1":   true,
	"qwerty":      true,
	"qwerty123":   true,
	"1234567890":  true,
	"123456789":   true,
	"11111111":    true,
	"admin123":    true,
	"iloveyou":    true,
	"welcome1":    true,
	"пароль123":   true,
	"autoimport":  true,
	"autoimport1": true,
	"avtoimport":  true,
	"1q2w3e4r":    true,
	"zaq12wsx":    true,
	"qazwsxedc":   true,
}

// ValidatePassword проверяет пароль по политике и возвращает
// человекочитаемую причину отказа.
func ValidatePassword(password string) error {
	runes := []rune(password)
	if len(runes) < MinPasswordLength {
		return fmt.Errorf("пароль должен содержать не менее %d символов", MinPasswordLength)
	}
	if len(runes) > MaxPasswordLength {
		return fmt.Errorf("пароль не должен превышать %d символов", MaxPasswordLength)
	}

	lower := strings.ToLower(password)
	if weakPasswords[lower] {
		return errors.New("этот пароль слишком распространён, выберите другой")
	}

	var hasLetter, hasDigit bool
	for _, r := range runes {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("пароль должен содержать и буквы, и цифры")
	}

	if isSequential(lower) {
		return errors.New("пароль не должен состоять из повторов или последовательности символов")
	}
	return nil
}

// isSequential отсекает "aaaaaaaaaa", "1234567890" и "abcdefghij".
func isSequential(s string) bool {
	runes := []rune(s)
	if len(runes) < 4 {
		return false
	}

	allSame := true
	ascending := true
	descending := true
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			allSame = false
		}
		if runes[i] != runes[i-1]+1 {
			ascending = false
		}
		if runes[i] != runes[i-1]-1 {
			descending = false
		}
	}
	return allSame || ascending || descending
}
