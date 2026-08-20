// Package auctions contiene el dominio, la persistencia y el HTTP de las subastas.
package auctions

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Status son los dos únicos estados posibles de una subasta.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusFinished Status = "FINISHED"
)

func (s Status) Valid() bool {
	return s == StatusActive || s == StatusFinished
}

// Límites de validación. Acotan la entrada sin pretender ser reglas de negocio.
const (
	maxTitleLength    = 200
	maxImageURLLength = 2048
	minDuration       = 10 * time.Second
	maxDuration       = 7 * 24 * time.Hour
)

var (
	ErrNotFound = errors.New("auction not found")
)

// ValidationError son los fallos de entrada (HTTP 400).
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// Auction es el modelo de dominio. Los importes son enteros en la unidad mínima
// (centavos) para evitar los errores de redondeo de la coma flotante.
type Auction struct {
	ID int64
	// CreatedBy es quien publicó la pieza: su dueño y, en esta plataforma, su
	// artista. No puede pujar en su propia subasta.
	CreatedBy        int64
	CreatedByName    string
	WinnerName       *string
	Title            string
	BasePrice        int64
	CurrentPrice     int64
	ImageURL         *string
	MinimumIncrement int64
	StartAt          time.Time
	EndAt            time.Time
	Status           Status
	WinnerID         *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// MinimumBid es el importe mínimo que aceptaría la siguiente puja.
// El motor de pujas del Milestone 4 valida contra este mismo cálculo.
func (a Auction) MinimumBid() int64 {
	return a.CurrentPrice + a.MinimumIncrement
}

// IsOpen indica si la subasta admite pujas: sigue ACTIVE y aún no ha vencido.
// El estado persistido no basta, porque entre el vencimiento y el cierre
// automático del Milestone 5 puede pasar un intervalo.
func (a Auction) IsOpen(now time.Time) bool {
	return a.Status == StatusActive && now.Before(a.EndAt)
}

// CreateInput son los únicos datos que el cliente puede aportar.
// current_price, status, start_at, end_at y winner_id los decide el servidor.
type CreateInput struct {
	CreatedBy        int64
	Title            string
	BasePrice        int64
	ImageURL         *string
	MinimumIncrement int64
	DurationSeconds  int64
}

// NewAuction es una subasta ya validada y con los valores calculados por el
// servidor, lista para persistirse.
type NewAuction struct {
	CreatedBy        int64
	Title            string
	BasePrice        int64
	CurrentPrice     int64
	ImageURL         *string
	MinimumIncrement int64
	StartAt          time.Time
	EndAt            time.Time
	Status           Status
}

// build valida la entrada del cliente y deriva el estado inicial de la subasta.
func (in CreateInput) build(now time.Time) (NewAuction, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return NewAuction{}, ValidationError{Message: "title is required"}
	}
	if len(title) > maxTitleLength {
		return NewAuction{}, ValidationError{Message: fmt.Sprintf("title must be at most %d characters", maxTitleLength)}
	}

	if in.BasePrice < 0 {
		return NewAuction{}, ValidationError{Message: "base_price must be zero or greater, in cents"}
	}
	if in.MinimumIncrement <= 0 {
		return NewAuction{}, ValidationError{Message: "minimum_increment must be greater than zero, in cents"}
	}

	duration := time.Duration(in.DurationSeconds) * time.Second
	if duration < minDuration || duration > maxDuration {
		return NewAuction{}, ValidationError{Message: fmt.Sprintf(
			"duration_seconds must be between %d and %d", int64(minDuration.Seconds()), int64(maxDuration.Seconds()))}
	}

	imageURL, err := normalizeImageURL(in.ImageURL)
	if err != nil {
		return NewAuction{}, err
	}

	startAt := now.UTC()

	return NewAuction{
		CreatedBy:        in.CreatedBy,
		Title:            title,
		BasePrice:        in.BasePrice,
		CurrentPrice:     in.BasePrice, // sin pujas, el precio actual es el precio base
		ImageURL:         imageURL,
		MinimumIncrement: in.MinimumIncrement,
		StartAt:          startAt,
		EndAt:            startAt.Add(duration),
		Status:           StatusActive,
	}, nil
}

// normalizeImageURL acepta la ausencia de imagen, pero si viene una exige que
// sea una URL http(s) absoluta.
func normalizeImageURL(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}

	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	if len(value) > maxImageURLLength {
		return nil, ValidationError{Message: fmt.Sprintf("image_url must be at most %d characters", maxImageURLLength)}
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, ValidationError{Message: "image_url must be an absolute http(s) URL"}
	}

	return &value, nil
}
