package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    string
		wantErr bool
	}{
		{name: "valid", header: "Bearer token", want: "token"},
		{name: "case insensitive", header: "bearer token", want: "token"},
		{name: "extra spaces", header: "  Bearer   token  ", want: "token"},
		{name: "missing token", header: "Bearer", wantErr: true},
		{name: "wrong scheme", header: "Basic token", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := bearerToken(test.header)
			if test.wantErr {
				if err == nil {
					t.Fatal("bearerToken() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("bearerToken() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("bearerToken() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPrincipalFrom(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := Principal{UserID: 42, Email: "test@example.com", Nickname: "tester"}
	context.Set(principalKey, want)

	got, ok := PrincipalFrom(context)
	if !ok {
		t.Fatal("PrincipalFrom() expected principal")
	}
	if got != want {
		t.Fatalf("PrincipalFrom() = %#v, want %#v", got, want)
	}
}
