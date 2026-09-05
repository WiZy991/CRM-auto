package httpx

import (
	"net/http"

	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/storage"
)

// UploadHandler принимает изображения дилера.
type UploadHandler struct {
	disk      *storage.Disk
	maxBytes  int64
	maxPixels int64
}

func NewUploadHandler(disk *storage.Disk, maxBytes, maxPixels int64) *UploadHandler {
	return &UploadHandler{disk: disk, maxBytes: maxBytes, maxPixels: maxPixels}
}

// Images — POST /api/v1/uploads/images
func (h *UploadHandler) Images(w http.ResponseWriter, r *http.Request) {
	if h.disk == nil {
		Error(w, r, &apierr.Error{
			Status:  http.StatusServiceUnavailable,
			Code:    "storage_unavailable",
			Message: "Хранилище файлов недоступно",
		})
		return
	}

	if err := r.ParseMultipartForm(h.maxBytes); err != nil {
		Error(w, r, apierr.BadRequest("Некорректное тело запроса"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		Error(w, r, apierr.Validation(map[string]string{"file": "выберите изображение"}))
		return
	}
	defer file.Close()

	if header.Size > h.maxBytes {
		Error(w, r, apierr.PayloadTooLarge("Файл слишком большой"))
		return
	}

	saved, err := h.disk.SaveImage(file, h.maxBytes, h.maxPixels)
	if err != nil {
		Error(w, r, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]any{
		"url":    saved.URL,
		"width":  saved.Width,
		"height": saved.Height,
	})
}
