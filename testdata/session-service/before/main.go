package main

import (
	"net/http"
	"example.test/session-service/handler"
)

func main() {
	http.HandleFunc("/users", handler.User)
}
