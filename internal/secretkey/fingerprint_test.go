package secretkey

import "testing"

func TestFingerprintIsStableAndDomainSeparated(t *testing.T) {
	const secret = "correct horse battery staple restore material"
	const want = "oi-secret-v1:0b81ff89fc8134dbc1788d6c8efd769b"
	if got := Fingerprint(secret); got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
	if got := Fingerprint(secret + "!"); got == want {
		t.Fatal("fingerprint did not change with the secret")
	}
	if got := Fingerprint(secret); got == secret {
		t.Fatal("fingerprint exposed the secret")
	}
}
