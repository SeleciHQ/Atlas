package storage

import (
	"encoding/base64"
	"encoding/hex"
)

// base64SHA256 converts a lowercase hex SHA-256 digest to the base64 form used
// by S3 checksum headers.
func base64SHA256(hexDigest string) string {
	raw, err := hex.DecodeString(hexDigest)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// hexFromBase64 converts an S3 checksum header back to lowercase hex.
func hexFromBase64(encoded string) string {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}
