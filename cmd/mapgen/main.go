package main

import (
	"log"
	"net/http"
	"os"

	"github.com/corpllm/mapgen/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	h := api.NewHandler()
	mux := http.NewServeMux()
	api.RegisterRoutes(mux, h)

	log.Printf("mapgen listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
