package state

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashContent returns a stable hex-encoded SHA-256 hash for content.
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// HashFileVersion returns a stable hex-encoded SHA-256 hash for a file version.
// The path is included so identical content at different paths yields different ids.
func HashFileVersion(path, content string) string {
	sum := sha256.Sum256([]byte(path + "\x00" + content))
	full := hex.EncodeToString(sum[:])
	// Use a short id for readability and to reduce prompt size.
	return full[:8]
}
