package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SecretFingerprint identifies an endpoint's signature secret without
// revealing it: HMAC-SHA256 of the secret keyed by the endpoint ID, so equal
// secrets on different endpoints don't have equal fingerprints. The edge
// returns it and the CLI computes it from hookly.yaml to see whether the
// secret needs updating.
func SecretFingerprint(endpointID, secret string) string {
	mac := hmac.New(sha256.New, []byte("hookly-secret-v1:"+endpointID))
	mac.Write([]byte(secret))
	return hex.EncodeToString(mac.Sum(nil))
}
