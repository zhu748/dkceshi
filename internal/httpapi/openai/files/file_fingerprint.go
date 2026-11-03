package files

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// Feed the original bytes directly to the hash instead of allocating another
// whole-file buffer. Keep the existing MIME/name separators and cache identity.
func inlineFileFingerprint(filename, contentType string, data []byte) string {
	digest := sha256.New()
	// SHA-256 writes always succeed; no I/O or partial writes occur here.
	_, _ = io.WriteString(digest, contentType+"\x00"+filename+"\x00")
	_, _ = digest.Write(data)
	return hex.EncodeToString(digest.Sum(nil))
}
