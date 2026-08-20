// Package bids contiene el motor de pujas: dominio, sincronización y la
// transacción que mantiene consistentes bids y auctions.
package bids

import (
	"errors"
	"fmt"
	"time"

	"bidcraft/internal/auctions"
)

// Errores de dominio. El handler los traduce a códigos HTTP.
var (
	ErrAuctionNotFound = errors.New("auction not found")
	ErrAuctionClosed   = errors.New("auction is closed")
	// ErrOwnAuction: quien publica una pieza no puede pujar por ella.
	ErrOwnAuction = errors.New("the owner cannot bid on their own auction")
)

// ValidationError son los fallos de entrada (HTTP 400).
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// TooLowError lleva el mínimo vigente para poder devolvérselo al cliente.
type TooLowError struct {
	Minimum int64
}

func (e TooLowError) Error() string {
	return fmt.Sprintf("minimum accepted bid is %s", FormatAmount(e.Minimum))
}

// FormatAmount pasa los centavos que guarda la base de datos a la forma con dos
// decimales que se muestra a las personas: 11000 -> "110.00".
func FormatAmount(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// Bid es una puja aceptada. Las rechazadas no se persisten.
type Bid struct {
	ID        int64
	AuctionID int64
	UserID    int64
	UserName  string
	Amount    int64
	CreatedAt time.Time
}

// Validate son las reglas de aceptación de una puja, como función pura para
// poder probarlas exhaustivamente sin base de datos.
//
// Se invoca DENTRO de la transacción, sobre la subasta ya bloqueada y con el
// reloj de PostgreSQL: leer el precio, validar y escribirlo es una única
// operación crítica.
func Validate(auction auctions.Auction, bidderID, amount int64, now time.Time) error {
	if amount <= 0 {
		return ValidationError{Message: "amount must be greater than zero, in cents"}
	}

	// La comprobación vive aquí, no en el frontend: es una regla de negocio.
	if auction.CreatedBy == bidderID {
		return ErrOwnAuction
	}

	// Una subasta vencida se rechaza aunque siga marcada ACTIVE: el cierre
	// automático puede no haber corrido todavía.
	if !auction.IsOpen(now) {
		return ErrAuctionClosed
	}

	if minimum := auction.MinimumBid(); amount < minimum {
		return TooLowError{Minimum: minimum}
	}

	return nil
}
