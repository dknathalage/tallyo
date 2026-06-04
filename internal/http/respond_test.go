package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, 200, map[string]string{"a": "b"})
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	if w.Code != 200 {
		t.Fatalf("code = %d", w.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body: %v", err)
	}
	if got["a"] != "b" {
		t.Fatalf("body = %v", got)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, 404, "nope")
	if w.Code != 404 {
		t.Fatalf("code = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"error":"nope"`) {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestDecodeJSONRejectsUnknownField(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"x","bogus":1}`))
	var dst in
	if err := DecodeJSON(r, &dst); err == nil {
		t.Fatal("unknown field must error")
	}
}

func TestDecodeJSONOK(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"x"}`))
	var dst in
	if err := DecodeJSON(r, &dst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dst.Name != "x" {
		t.Fatalf("dst = %+v", dst)
	}
}
