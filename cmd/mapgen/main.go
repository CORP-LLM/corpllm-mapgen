package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/corpllm/mapgen/api"
	"github.com/corpllm/mapgen/internal/terrain"
)

const usage = `mapgen — procedural terrain generator

Usage:
  mapgen                 start HTTP server + browser editor (default)
  mapgen serve           same as above
  mapgen generate        read config JSON on stdin, emit terrain JSON on stdout
  mapgen schema          emit the JSON Schema for the terrain response
  mapgen help            print this message

Environment:
  PORT                   HTTP port for serve mode (default 8080)

Examples:
  mapgen                                             # dev: editor + API on :8080
  echo '{"seed":42}' | mapgen generate > map.json    # headless baking
  mapgen schema > terrain.schema.json                # copy schema locally
`

func main() {
	cmd := "serve"
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		runServer()
	case "generate":
		if err := runGenerate(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "mapgen generate:", err)
			os.Exit(1)
		}
	case "schema":
		if err := runSchema(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "mapgen schema:", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func runServer() {
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

// runGenerate: read a terrain.Config on stdin, write the resulting Terrain
// on stdout. Missing fields are defaulted; validation errors surface
// non-zero.
func runGenerate(in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	cfg := &terrain.Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, cfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	tm, err := terrain.Generate(cfg)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(tm)
}

func runSchema(out io.Writer) error {
	b, err := os.ReadFile("./schema/terrain.schema.json")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	_, err = out.Write(b)
	return err
}
