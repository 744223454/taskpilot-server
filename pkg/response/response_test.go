package response

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeIncludesNullData(t *testing.T) {
	body, err := json.Marshal(Envelope{
		Code:    10005,
		Message: "internal server error",
		Data:    nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	const want = `{"code":10005,"message":"internal server error","data":null}`
	if string(body) != want {
		t.Fatalf("json.Marshal() = %s, want %s", body, want)
	}
}
