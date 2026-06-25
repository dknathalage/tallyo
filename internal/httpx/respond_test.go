package httpx

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/apperr"
)

func TestWriteServiceError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantCode  int
		wantWrote bool
	}{
		{"nil falls through", nil, 200, false},
		{"not found", apperr.ErrNotFound, 404, true},
		{"conflict", apperr.ErrConflict, 409, true},
		{"validation", &apperr.ValidationError{Errors: []apperr.FieldError{{Field: "name", Message: "required"}}}, 422, true},
		{"unknown", errors.New("boom"), 500, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			got := WriteServiceError(rec, c.err)
			if got != c.wantWrote {
				t.Fatalf("wrote: want %v got %v", c.wantWrote, got)
			}
			if c.wantWrote && rec.Code != c.wantCode {
				t.Fatalf("status: want %d got %d", c.wantCode, rec.Code)
			}
		})
	}
}

func TestDecodeJSONRejectsBadBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader("{not json"))
	var dst struct{ A string }
	if err := DecodeJSON(r, &dst); err == nil {
		t.Fatal("want error for malformed body, got nil")
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"nope":1}`))
	var dst struct {
		A string `json:"a"`
	}
	if err := DecodeJSON(r, &dst); err == nil {
		t.Fatal("want error for unknown field, got nil")
	}
}
