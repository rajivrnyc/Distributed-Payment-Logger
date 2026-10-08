package main

import (
	"log"
	"net/http"
	"os"

	"example.com/payments/internal/app"
)

func main() {
	srv := app.NewServer()
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	log.Printf("payments-api listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatal(err)
	}
}
