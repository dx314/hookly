package crypto

import "testing"

func TestSecretFingerprint(t *testing.T) {
	a := SecretFingerprint("ep1", "secret")
	if len(a) != 64 || a != SecretFingerprint("ep1", "secret") {
		t.Fatalf("fingerprint %q not a stable hex SHA-256", a)
	}
	if a == SecretFingerprint("ep2", "secret") {
		t.Error("same secret on another endpoint has the same fingerprint")
	}
	if a == SecretFingerprint("ep1", "secret2") {
		t.Error("different secrets have the same fingerprint")
	}
}
