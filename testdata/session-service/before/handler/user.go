package handler

import "net/http"

func User(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
