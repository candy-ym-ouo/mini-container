package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIErrorJSONContract(t *testing.T) {
	data, err := json.Marshal(ErrorEnvelope{Error: APIError{Code: "NOT_FOUND", Message: "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"error":{"code":"NOT_FOUND","message":"missing"}}`
	if string(data) != want {
		t.Fatalf("error JSON = %s", data)
	}
}

func TestDecodeJSONRejectsTrailingValue(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"cmd":["true"]} {}`))
	request.Header.Set("Content-Type", "application/json")
	var body ExecReq
	if err := decodeJSON(request, &body); err == nil {
		t.Fatal("trailing JSON value accepted")
	}
}
