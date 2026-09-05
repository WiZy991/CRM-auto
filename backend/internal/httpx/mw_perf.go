package httpx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Gzip сжимает JSON-ответы, если клиент явно просит gzip.
//
// Сжатие включается только для текстовых типов: фото уже сжаты, повторный
// gzip увеличивает тело и тратит CPU. Порог в байтах отсекает крошечные
// ответы, где заголовок gzip длиннее самой экономии.
func Gzip(next http.Handler) http.Handler {
	pool := sync.Pool{
		New: func() any {
			w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
			return w
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/uploads/") || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		wrapper := &gzipResponse{ResponseWriter: w, pool: &pool}
		defer wrapper.close()
		next.ServeHTTP(wrapper, r)
	})
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		encoding, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(encoding, "gzip") {
			return true
		}
	}
	return false
}

type gzipResponse struct {
	http.ResponseWriter
	pool   *sync.Pool
	gz     *gzip.Writer
	status int
}

func (g *gzipResponse) WriteHeader(status int) {
	g.status = status
	if status == http.StatusNoContent || status == http.StatusNotModified {
		g.ResponseWriter.WriteHeader(status)
		return
	}
	g.ensureGzip()
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipResponse) Write(p []byte) (int, error) {
	if g.status == http.StatusNoContent || g.status == http.StatusNotModified {
		return g.ResponseWriter.Write(p)
	}
	g.ensureGzip()
	if g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipResponse) ensureGzip() {
	if g.gz != nil {
		return
	}
	header := g.Header()
	if header.Get("Content-Encoding") != "" {
		return
	}
	ctype := header.Get("Content-Type")
	if ctype != "" && !compressibleType(ctype) {
		return
	}
	writer, _ := g.pool.Get().(*gzip.Writer)
	writer.Reset(g.ResponseWriter)
	g.gz = writer
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	header.Add("Vary", "Accept-Encoding")
}

func (g *gzipResponse) close() {
	if g.gz == nil {
		return
	}
	_ = g.gz.Close()
	g.pool.Put(g.gz)
	g.gz = nil
}

func compressibleType(ctype string) bool {
	switch {
	case strings.HasPrefix(ctype, "application/json"):
		return true
	case strings.HasPrefix(ctype, "text/"):
		return true
	case strings.HasPrefix(ctype, "application/javascript"):
		return true
	default:
		return false
	}
}

// ETagOnGET считает отпечаток тела GET-ответа и отвечает 304, если клиент
// уже держит ту же копию. Считается по несжатому JSON: gzip снаружи не
// должен менять ETag от запроса к запросу.
func ETagOnGET(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		buf := &etagBuffer{ResponseWriter: w, buf: bytes.NewBuffer(nil)}
		next.ServeHTTP(buf, r)

		status := buf.status
		if status == 0 {
			status = http.StatusOK
		}
		if status != http.StatusOK || buf.buf.Len() == 0 || buf.skip {
			buf.flush(status)
			return
		}

		sum := sha256.Sum256(buf.buf.Bytes())
		etag := `"` + hex.EncodeToString(sum[:16]) + `"`
		w.Header().Set("ETag", etag)
		if noneMatch := r.Header.Get("If-None-Match"); noneMatch != "" && strings.Contains(noneMatch, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		buf.flush(status)
	})
}

type etagBuffer struct {
	http.ResponseWriter
	buf    *bytes.Buffer
	status int
	skip   bool
}

func (e *etagBuffer) WriteHeader(status int) {
	e.status = status
	if status != http.StatusOK {
		e.skip = true
		e.ResponseWriter.WriteHeader(status)
	}
}

func (e *etagBuffer) Write(p []byte) (int, error) {
	if e.skip {
		return e.ResponseWriter.Write(p)
	}
	return e.buf.Write(p)
}

func (e *etagBuffer) flush(status int) {
	if e.skip {
		return
	}
	if e.status == 0 || e.status == http.StatusOK {
		e.ResponseWriter.WriteHeader(status)
	}
	_, _ = e.ResponseWriter.Write(e.buf.Bytes())
}
