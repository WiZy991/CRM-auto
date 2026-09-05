// Package storage сохраняет загруженные изображения на диск.
//
// Файл принимается только после проверки сигнатуры и полной перекодировки:
// имя, Content-Type и EXIF исходника в выдачу не попадают.
package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/autoimport/crm/internal/pkg/apierr"
)

// Image — результат сохранённого файла.
type Image struct {
	URL    string
	Width  int
	Height int
}

// Disk хранит файлы в локальном каталоге и отдаёт их по префиксу /uploads/.
type Disk struct {
	dir string
}

func NewDisk(dir string) (*Disk, error) {
	if dir == "" {
		dir = "./storage/uploads"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("каталог загрузок: %w", err)
	}
	return &Disk{dir: dir}, nil
}

func (d *Disk) Dir() string { return d.dir }

// SaveImage принимает JPEG или PNG, перекодирует в JPEG и пишет под случайным именем.
func (d *Disk) SaveImage(r io.Reader, maxBytes, maxPixels int64) (*Image, error) {
	limited := io.LimitReader(r, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, apierr.BadRequest("Не удалось прочитать файл")
	}
	if int64(len(raw)) > maxBytes {
		return nil, apierr.PayloadTooLarge("Файл слишком большой")
	}
	if len(raw) < 24 {
		return nil, apierr.BadRequest("Файл слишком короткий, чтобы быть изображением")
	}
	if !allowedMagic(raw) {
		return nil, apierr.BadRequest("Допустимы только JPEG и PNG")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, apierr.BadRequest("Файл не удалось разобрать как изображение")
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if pixels <= 0 || pixels > maxPixels {
		return nil, apierr.BadRequest("Разрешение изображения слишком большое")
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, apierr.BadRequest("Файл не удалось декодировать")
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, apierr.Internal(fmt.Errorf("перекодирование изображения: %w", err))
	}

	name, err := randomName()
	if err != nil {
		return nil, apierr.Internal(err)
	}
	path := filepath.Join(d.dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o640); err != nil {
		return nil, apierr.Internal(fmt.Errorf("запись файла: %w", err))
	}

	return &Image{
		URL:    "/uploads/" + name,
		Width:  cfg.Width,
		Height: cfg.Height,
	}, nil
}

func allowedMagic(raw []byte) bool {
	if len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 && raw[2] == 0xff {
		return true
	}
	if len(raw) >= 8 && string(raw[:8]) == "\x89PNG\r\n\x1a\n" {
		return true
	}
	return false
}

func randomName() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("имя файла: %w", err)
	}
	return hex.EncodeToString(buf) + ".jpg", nil
}

// FileServer отдаёт сохранённые файлы без листинга каталога.
func FileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}
			clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if clean == "private" || strings.HasPrefix(clean, "private/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fs.ServeHTTP(w, r)
	})
}
