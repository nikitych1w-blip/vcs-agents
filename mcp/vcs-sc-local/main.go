package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	repoRoot := getEnv("REPO_ROOT", "/repo")
	port := getEnv("PORT", "8081")

	repo := &RepoWriter{Root: repoRoot}
	h := NewHandler(repo)

	http.HandleFunc("/mcp", h.Handle)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("vcs-sc-local-mcp | port=%s repo=%s", port, repoRoot)
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
