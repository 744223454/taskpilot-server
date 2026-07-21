package auth

import (
	"errors"
	"testing"
	"time"
)

func TestManagerGenerateAndParseToken(t *testing.T) {
	baseTime := time.Unix(1_700_000_000, 0)
	manager := NewManager("secret", 3600)
	manager.now = func() time.Time { return baseTime }

	token, err := manager.GenerateToken(Claims{
		UserID:   42,
		Email:    "user@example.com",
		Nickname: "taskpilot",
	})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := manager.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("claims.UserID = %d, want 42", claims.UserID)
	}
	if claims.Email != "user@example.com" {
		t.Fatalf("claims.Email = %q, want user@example.com", claims.Email)
	}
	if claims.Nickname != "taskpilot" {
		t.Fatalf("claims.Nickname = %q, want taskpilot", claims.Nickname)
	}
}

func TestManagerParseTokenExpired(t *testing.T) {
	baseTime := time.Unix(1_700_000_000, 0)
	manager := NewManager("secret", 1)
	manager.now = func() time.Time { return baseTime }

	token, err := manager.GenerateToken(Claims{UserID: 1})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	manager.now = func() time.Time { return baseTime.Add(2 * time.Second) }
	_, err = manager.ParseToken(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("ParseToken() error = %v, want %v", err, ErrExpiredToken)
	}
}

func TestManagerParseTokenInvalidSignature(t *testing.T) {
	manager := NewManager("secret", 3600)
	manager.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	token, err := manager.GenerateToken(Claims{UserID: 1})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	other := NewManager("other-secret", 3600)
	other.now = manager.now
	_, err = other.ParseToken(token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseToken() error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestManagerValidateConfig(t *testing.T) {
	manager := NewManager("", 0)
	if err := manager.Validate(); !errors.Is(err, ErrInvalidJWTConfig) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidJWTConfig)
	}
}
