package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SavedFile — файл, записанный вне публичной раздачи /uploads/.
type SavedFile struct {
	RelPath  string
	MimeType string
	Bytes    int
}

// SavePrivate пишет байты в каталог, который FileServer не отдаёт.
//
// Сформированные договоры содержат паспорт и адрес. Их нельзя класть рядом
// с обложками объявлений: иначе угаданный URL открывал бы ПДн без сессии.
func (d *Disk) SavePrivate(subdir, ext string, payload []byte, mime string) (*SavedFile, error) {
	if d == nil {
		return nil, fmt.Errorf("хранилище не настроено")
	}
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "bin"
	}
	name, err := randomHexName()
	if err != nil {
		return nil, err
	}

	rel := pathJoin("private", subdir, name+"."+ext)
	full := filepath.Join(d.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, fmt.Errorf("каталог закрытых файлов: %w", err)
	}
	if err := os.WriteFile(full, payload, 0o640); err != nil {
		return nil, fmt.Errorf("запись закрытого файла: %w", err)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return &SavedFile{RelPath: rel, MimeType: mime, Bytes: len(payload)}, nil
}

// Resolve возвращает абсолютный путь к файлу сделки, если он лежит на диске.
//
// Публичные картинки остаются по /uploads/…, закрытые документы — по
// относительному private/…. Чужой префикс (http://) не резолвится.
func (d *Disk) Resolve(fileURL string) (string, bool) {
	if d == nil || fileURL == "" {
		return "", false
	}

	var rel string
	switch {
	case strings.HasPrefix(fileURL, "/uploads/"):
		rel = strings.TrimPrefix(fileURL, "/uploads/")
	case strings.HasPrefix(fileURL, "private/"):
		rel = fileURL
	default:
		return "", false
	}

	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", false
	}
	full := filepath.Join(d.dir, clean)
	if !strings.HasPrefix(full, filepath.Clean(d.dir)+string(os.PathSeparator)) &&
		full != filepath.Clean(d.dir) {
		return "", false
	}
	return full, true
}

func randomHexName() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("имя файла: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func pathJoin(parts ...string) string {
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		trimmed = append(trimmed, part)
	}
	return strings.Join(trimmed, "/")
}
