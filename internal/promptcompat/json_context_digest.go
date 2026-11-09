package promptcompat

import (
	"ds2api/internal/jsonvalue"
	"encoding/hex"
)

func digestJSONContext(value any) (ContextDigest, error) {
	size, sum, err := jsonvalue.JSONFingerprint(value)
	if err != nil {
		return ContextDigest{}, err
	}
	return ContextDigest{Bytes: size, SHA256: hex.EncodeToString(sum[:])}, nil
}
