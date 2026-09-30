package managementasset

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestPanelNeverFetchesOrReplacesLocalAsset(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		t.Error("panel attempted an external download")
	}))
	defer server.Close()
	dir := t.TempDir()
	StartAutoUpdater(t.Context(), filepath.Join(dir, "config.yaml"))
	if EnsureLatestManagementHTML(t.Context(), dir, "", server.URL) {
		t.Fatal("missing local asset must fail closed")
	}
	content := []byte("<html>owned panel</html>")
	path := filepath.Join(dir, ManagementFileName)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if !EnsureLatestManagementHTML(t.Context(), dir, "", server.URL) {
		t.Fatal("local asset should be available")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(content) || requests.Load() != 0 {
		t.Fatalf("asset changed or fetched: err=%v requests=%d", err, requests.Load())
	}
}

func TestPanelPathOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MANAGEMENT_STATIC_PATH", filepath.Join(dir, ManagementFileName))
	if got := FilePath("ignored.yaml"); got != filepath.Join(dir, ManagementFileName) {
		t.Fatalf("unexpected panel path: %s", got)
	}
}
