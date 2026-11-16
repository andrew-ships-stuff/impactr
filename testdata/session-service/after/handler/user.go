package handler

import (
	"net/http"
	"example.test/session-service/session"
)

func User(w http.ResponseWriter, r *http.Request) {
	if !session.Validate(r.Header.Get("Authorization")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}
