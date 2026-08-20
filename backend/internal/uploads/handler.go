// Package uploads guarda en disco las imágenes que suben los usuarios.
//
// Es lo más simple que cumple el requisito: los archivos van a una carpeta del
// servidor y se sirven como estáticos. Para una prueba técnica no hace falta un
// almacenamiento externo.
package uploads

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"bidcraft/internal/httpx"
)

const (
	maxUploadBytes = 5 << 20 // 5 MB
	formField      = "file"
)

// allowedTypes son los formatos de imagen aceptados y su extensión.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

type Handler struct {
	dir string
}

// NewHandler crea la carpeta de subidas si no existe.
func NewHandler(dir string) (*Handler, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}

	return &Handler{dir: dir}, nil
}

// FileServer sirve las imágenes ya guardadas.
func (h *Handler) FileServer() http.Handler {
	return http.FileServer(http.Dir(h.dir))
}

// Upload recibe una imagen y devuelve la ruta con la que referenciarla.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	file, header, err := r.FormFile(formField)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error",
			"Send an image in the \"file\" field, up to 5 MB")
		return
	}
	defer file.Close()

	// El tipo se deduce del contenido, no de lo que diga el cliente.
	head := make([]byte, 512)
	read, err := file.Read(head)
	if err != nil && err != io.EOF {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "Could not read the image")
		return
	}

	extension, ok := allowedTypes[http.DetectContentType(head[:read])]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "validation_error",
			"Only JPG, PNG, WEBP or GIF images are accepted")
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
		return
	}

	name, err := randomName(extension)
	if err != nil {
		log.Printf("uploads: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
		return
	}

	destination, err := os.Create(filepath.Join(h.dir, name))
	if err != nil {
		log.Printf("uploads: create file: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
		return
	}
	defer destination.Close()

	if _, err := io.Copy(destination, file); err != nil {
		log.Printf("uploads: write file: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error",
			"The image could not be stored")
		return
	}

	log.Printf("image uploaded: %s (%s)", name, prettySize(header.Size))
	httpx.JSON(w, http.StatusCreated, map[string]string{"path": "/uploads/" + name})
}

// randomName evita colisiones y que el nombre original controle la ruta.
func randomName(extension string) (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate file name: %w", err)
	}

	return hex.EncodeToString(buffer) + extension, nil
}

func prettySize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1<<20 {
		return fmt.Sprintf("%.0f KB", float64(bytes)/1024)
	}

	return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
}
