package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// computeSignature is a test helper that produces the expected
// X-Hub-Signature-256 header value for a given body and secret.
// Format matches GitHub: "sha256=<hex>". This duplicates the
// algorithm used in validateSignature — by design — so tests verify
// validateSignature agrees with the HMAC-SHA256 spec rather than
// with any particular implementation detail.
func computeSignature(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func TestValidateSignature(t *testing.T) {
	body := []byte(`{"action":"opened","issue":{"number":42}}`)
	secret := "super-secret-webhook-key"
	valid := computeSignature(body, secret)

	tests := []struct {
		name      string
		body      []byte
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      body,
			signature: valid,
			secret:    secret,
			want:      true,
		},
		{
			name:      "wrong secret",
			body:      body,
			signature: valid,
			secret:    "wrong-secret",
			want:      false,
		},
		{
			name:      "tampered body",
			body:      []byte(`{"action":"closed","issue":{"number":42}}`),
			signature: valid,
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty signature",
			body:      body,
			signature: "",
			secret:    secret,
			want:      false,
		},
		{
			name:      "missing sha256= prefix",
			body:      body,
			signature: valid[len("sha256="):], // strip the prefix
			secret:    secret,
			want:      false,
		},
		{
			name: "wrong hash same length",
			body: body,
			// 32 zero bytes hex-encoded = 64 hex chars = same length as a real
			// SHA-256 hash. Guards against any short-circuit that would accept
			// wrong hex of the right length.
			signature: "sha256=" + hex.EncodeToString(make([]byte, 32)),
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty body with valid-for-empty signature",
			body:      []byte{},
			signature: computeSignature([]byte{}, secret),
			secret:    secret,
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateSignature(tc.body, tc.signature, tc.secret)
			if got != tc.want {
				t.Errorf("validateSignature(body, %q, %q) = %v, want %v",
					tc.signature, tc.secret, got, tc.want)
			}
		})
	}
}
