package failure

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// maxRunIDLen bounds a run id so a marker line stays short.
const maxRunIDLen = 128

// NewRunID returns a new run id, unique per call. It is generated once per
// terminal run and reused across retries of the same punch-out. Its characters
// are within [A-Za-z0-9._-] so it can sit safely inside the marker.
func NewRunID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on supported platforms. Fall back to the
		// clock so a caller still gets a valid, almost certainly unique id.
		return "t" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// ValidRunID reports whether id is 1 to 128 characters from [A-Za-z0-9._-].
func ValidRunID(id string) bool {
	if id == "" || len(id) > maxRunIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
