package v1

import (
	"net/http"
	"os"
	"path/filepath"
)

// MARK: handleWebUI
// Return the user to the index.html file.
// MARK: handleWebUI
func (a *APIServer) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	webRoot := a.cfg.Server.WebRoot
	indexPath := filepath.Join(webRoot, "index.html")

	if _, err := os.Stat(indexPath); err != nil {
		a.logger.Error("Web index.html not found", "path", indexPath, "web_root", webRoot, "error", err)
		http.Error(w, "Web interface not available", http.StatusInternalServerError)
		return
	}

	http.ServeFile(w, r, indexPath)
}
