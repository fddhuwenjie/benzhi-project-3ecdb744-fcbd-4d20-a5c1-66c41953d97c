package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/workflow"
)

func TestJSONRoutesRejectWrongMediaType(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	server := New(workflow.New(repository)).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/drills", strings.NewReader(`{}`))
	request = request.WithContext(context.Background())
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", response.Code)
	}
}
