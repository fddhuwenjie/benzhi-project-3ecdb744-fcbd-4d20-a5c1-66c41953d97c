package healthstorageunavailable

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/web"
	"shelter-drill-gate/internal/workflow"
)

func TestHealthRejectsUnavailableStorage(t *testing.T) {
	repository, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	handler := web.New(workflow.New(repository)).Handler()
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code == http.StatusOK {
		t.Fatalf("TestHealthRejectsUnavailableStorage: closed SQLite dependency was reported healthy: %s", response.Body.String())
	}
}
