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
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	log.Printf("mapgen listening on :%s — ui: http://localhost:%s/", port, port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
