package auth

import "testing"

func TestBearerTokenAcceptsCaseInsensitiveScheme(t *testing.T) {
	token, err := bearerToken("bearer abc.def.ghi")
	if err != nil {
		t.Fatalf("bearerToken() error = %v", err)
	}
	if token != "abc.def.ghi" {
		t.Fatalf("bearerToken() token = %q, want abc.def.ghi", token)
	}
}

func TestBearerTokenAcceptsExtraWhitespace(t *testing.T) {
	token, err := bearerToken("  Bearer   abc.def.ghi  ")
	if err != nil {
		t.Fatalf("bearerToken() error = %v", err)
	}
	if token != "abc.def.ghi" {
		t.Fatalf("bearerToken() token = %q, want abc.def.ghi", token)
	}
}

func TestBearerTokenRejectsMalformedHeader(t *testing.T) {
	if _, err := bearerToken("Bearer"); err == nil {
		t.Fatal("bearerToken() expected error for malformed header")
	}
}
