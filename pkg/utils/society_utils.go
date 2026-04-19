package utils

import (
	"crypto/rand"
	"strings"
)

const (
	societyCodeCharset  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	societyCodeLength   = 10
	societyPrefixLen    = 5
	pincodeSuffixLen    = 4
	societyRandomSuffix = 1
)

// GenerateSocietyCode generates a deterministic-prefix + random-suffix society code.
// Format: [5 chars from name+city+state][4 digits from pincode][1 random alphanumeric]
// Total: 10 characters, uppercase alphanumeric.
// Callers should handle uniqueness constraint violations by retrying.
func GenerateSocietyCode(name, city, state, pincode string) string {
	prefix := buildPrefix(name, city, state)
	pinPart := buildPinPart(pincode)
	randomPart := mustRandomString(societyRandomSuffix)
	return prefix + pinPart + randomPart
}

// buildPrefix extracts the first 5 uppercase alphanumeric characters
// from the concatenation of name+city+state, padding with 'X' if needed.
func buildPrefix(name, city, state string) string {
	combined := strings.ToUpper(name + city + state)

	// Keep only alphanumeric characters to avoid special chars in code
	var b strings.Builder
	for _, r := range combined {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() == societyPrefixLen {
				break
			}
		}
	}

	result := b.String()
	for len(result) < societyPrefixLen {
		result += "X"
	}
	return result
}

// buildPinPart extracts the last 4 digits of the pincode, left-padding with '0' if needed.
func buildPinPart(pincode string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, pincode)

	if len(digits) >= pincodeSuffixLen {
		return digits[len(digits)-pincodeSuffixLen:]
	}
	for len(digits) < pincodeSuffixLen {
		digits = "0" + digits
	}
	return digits
}

// mustRandomString generates a cryptographically random alphanumeric string of given length.
// Panics only if the system's random source fails, which is an unrecoverable condition.
func mustRandomString(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("utils: crypto/rand unavailable: " + err.Error())
	}
	for i := range b {
		b[i] = societyCodeCharset[int(b[i])%len(societyCodeCharset)]
	}
	return string(b)
}