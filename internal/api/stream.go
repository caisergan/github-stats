package api

import "net/http"

func (s *Server) syncStream(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
