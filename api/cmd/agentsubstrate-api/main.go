package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/httpapi"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/registry"
)

func main() {
	addr := getenv("ADDR", ":8080")
	dir := getenv("DATA_DIR", "data")
	reg, err := registry.Open(dir)
	if err != nil {
		log.Fatalf("open registry: %v", err)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(reg),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("AgentSubstrateAPI listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
