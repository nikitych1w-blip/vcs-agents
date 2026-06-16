package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	vaultRoot := getEnv("VAULT_ROOT", "/vcs-vault-local")
	port := getEnv("PORT", "8080")

	vault := &VaultReader{Root: vaultRoot}
	h := NewHandler(vault)

	// Streamable HTTP transport (MCP spec 2025-03-26)
	// POST /mcp  — single endpoint for all JSON-RPC messages
	http.HandleFunc("/mcp", h.Handle)

	// health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("vcs-vault-local-mcp | port=%s vcs-vault-local=%s", port, vaultRoot)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
