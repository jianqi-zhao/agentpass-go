package agentpass

// Shared credit validation and idempotency helpers for the active SDK APIs.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
)

func validCreditValue(value float64, allowZero bool) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || (!allowZero && value == 0) {
		return false
	}
	return math.Abs(value*100) <= 9_007_199_254_740_991 &&
		math.Abs(value*100-math.Round(value*100)) < 1e-8
}

// NewIdempotencyKey returns a UUID-shaped cryptographically random request key.
func NewIdempotencyKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", errors.New("agentpass: generate idempotency key: " + err.Error())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
