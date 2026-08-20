package bids

import (
	"errors"
	"testing"
	"time"

	"bidcraft/internal/auctions"
)

const (
	owner   = int64(1)
	bidder  = int64(2)
	another = int64(3)
)

var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// Subasta abierta del usuario 1: precio 10000, incremento 1000, mínimo 11000.
func openAuction() auctions.Auction {
	return auctions.Auction{
		ID:               10,
		CreatedBy:        owner,
		CurrentPrice:     10000,
		MinimumIncrement: 1000,
		Status:           auctions.StatusActive,
		EndAt:            now.Add(5 * time.Minute),
	}
}

func TestOwnerCannotBidOnTheirOwnAuction(t *testing.T) {
	if err := Validate(openAuction(), owner, 11000, now); !errors.Is(err, ErrOwnAuction) {
		t.Errorf("Validate() error = %v, want ErrOwnAuction", err)
	}
}

func TestOtherUsersCanBid(t *testing.T) {
	for _, userID := range []int64{bidder, another} {
		if err := Validate(openAuction(), userID, 11000, now); err != nil {
			t.Errorf("Validate(user %d) error = %v, want nil", userID, err)
		}
	}
}

// La regla es del backend: se cumple aunque el importe sea generoso o la puja
// llegue desde cualquier cliente.
func TestOwnerRuleAppliesWhateverTheAmount(t *testing.T) {
	for _, amount := range []int64{11000, 50000, 999999} {
		if err := Validate(openAuction(), owner, amount, now); !errors.Is(err, ErrOwnAuction) {
			t.Errorf("Validate(%d) error = %v, want ErrOwnAuction", amount, err)
		}
	}
}

func TestValidateStillRejectsLowBidsAndClosedAuctions(t *testing.T) {
	var tooLow TooLowError
	if err := Validate(openAuction(), bidder, 10999, now); !errors.As(err, &tooLow) {
		t.Errorf("Validate(below minimum) error = %v, want TooLowError", err)
	}

	closed := openAuction()
	closed.Status = auctions.StatusFinished
	if err := Validate(closed, bidder, 99999, now); !errors.Is(err, ErrAuctionClosed) {
		t.Errorf("Validate(closed) error = %v, want ErrAuctionClosed", err)
	}
}

// Los importes se guardan en centavos y se muestran con dos decimales.
func TestFormatAmount(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{11000, "110.00"},
		{12550, "125.50"},
		{100, "1.00"},
		{5, "0.05"},
		{0, "0.00"},
	}

	for _, tt := range tests {
		if got := FormatAmount(tt.cents); got != tt.want {
			t.Errorf("FormatAmount(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}

func TestTooLowErrorMessageUsesDecimals(t *testing.T) {
	if got := (TooLowError{Minimum: 11000}).Error(); got != "minimum accepted bid is 110.00" {
		t.Errorf("Error() = %q", got)
	}
}
