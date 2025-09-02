package v1

import (
	"net/http"
)

// MARK: handleWebUI
// Return the user to the index.html file.
func (a *APIServer) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "./web/index.html")
}
