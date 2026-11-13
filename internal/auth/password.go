package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"golang.org/x/crypto/bcrypt"
	"log/slog"
	"strings"
)

// A fixed-size prehash supports long UTF-8 passwords without bcrypt's 72-byte
// truncation limit. The explicit prefix distinguishes it from legacy hashes.
const passwordHashPrefix = "bcrypt-sha256:"

func HashAdminPassword(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(sum[:])), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("admin password hashing failed", "error", err)
		return ""
	}
	return passwordHashPrefix + string(hash)
}

func verifyAdminPasswordHash(candidate, encoded string) bool {
	encoded = strings.TrimSpace(encoded)
	if strings.HasPrefix(encoded, passwordHashPrefix) {
		sum := sha256.Sum256([]byte(candidate))
		return bcrypt.CompareHashAndPassword([]byte(strings.TrimPrefix(encoded, passwordHashPrefix)), []byte(hex.EncodeToString(sum[:]))) == nil
	}
	encoded = strings.ToLower(encoded)
	if strings.HasPrefix(encoded, "sha256:") {
		sum := sha256.Sum256([]byte(candidate))
		return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(strings.TrimPrefix(encoded, "sha256:"))) == 1
	}
	return encoded != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(encoded)) == 1
}
