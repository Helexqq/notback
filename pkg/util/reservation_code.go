package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func Reservation_сode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ValidateReservationCode(code string) error {
	if len(code) != 32 {
		return fmt.Errorf("invalid length %d (expected 32)", len(code))
	}
	if _, err := hex.DecodeString(code); err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	return nil
}
