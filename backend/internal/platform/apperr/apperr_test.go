package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestFromMapsKinds(t *testing.T) {
	cases := []struct {
		kind   Kind
		status int
	}{
		{KindInvalid, http.StatusBadRequest},
		{KindUnauthenticated, http.StatusUnauthorized},
		{KindForbidden, http.StatusForbidden},
		{KindNotFound, http.StatusNotFound},
		{KindConflict, http.StatusConflict},
		{KindUnavailable, http.StatusServiceUnavailable},
		{KindInternal, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		problem := From(New(tc.kind, "test_code", "safe message"))
		if problem.Status != tc.status {
			t.Errorf("kind %s: status = %d, want %d", tc.kind, problem.Status, tc.status)
		}
		expectedCode := "test_code"
		if tc.kind == KindInternal {
			expectedCode = "internal_error"
		}
		if problem.Code != expectedCode {
			t.Errorf("kind %s: code = %q, want %q", tc.kind, problem.Code, expectedCode)
		}
	}
}

func TestInternalErrorIsRedacted(t *testing.T) {
	err := New(KindInternal, "db_write_failed", "sensitive detail").WithCause(fmt.Errorf("secret connection string"))
	problem := From(err)
	if problem.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d", problem.Status)
	}
	if problem.Detail == "sensitive detail" || problem.Code != "internal_error" {
		t.Fatalf("internal error leaked detail: %+v", problem)
	}
}

func TestUnknownErrorIsInternal(t *testing.T) {
	problem := From(errors.New("random failure"))
	if problem.Status != http.StatusInternalServerError || problem.Code != "internal_error" {
		t.Fatalf("unexpected mapping: %+v", problem)
	}
	if !IsInternal(errors.New("random failure")) {
		t.Fatal("unknown errors must be treated as internal")
	}
}

func TestFieldsArePreserved(t *testing.T) {
	err := New(KindInvalid, "validation_failed", "Request is invalid").WithField("email", "must be an email address")
	problem := From(err)
	if problem.Errors["email"] != "must be an email address" {
		t.Fatalf("field errors missing: %+v", problem)
	}
}
