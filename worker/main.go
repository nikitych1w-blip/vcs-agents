package main

import (
	"log"
	"os"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	vcllm "github.com/nikitych1w-blip/vcs-agents/worker/llm"
	"github.com/nikitych1w-blip/vcs-agents/worker/sc"
	"github.com/nikitych1w-blip/vcs-agents/worker/vault"
)

func main() {
	temporalAddr := env("TEMPORAL_ADDRESS", "localhost:7233")
	temporalNS   := env("TEMPORAL_NAMESPACE", "default")
	litellmURL   := env("LITELLM_BASE_URL", "http://localhost:4000")
	litellmKey   := env("LITELLM_API_KEY", "")
	vaultMCPURL  := env("VAULT_MCP_URL", "http://localhost:8080/mcp")
	scMCPURL     := env("SC_LOCAL_MCP_URL", "")
	defaultModel := env("DEFAULT_MODEL", "company-main")

	c, err := client.Dial(client.Options{
		HostPort:  temporalAddr,
		Namespace: temporalNS,
	})
	if err != nil {
		log.Fatalf("temporal dial: %v", err)
	}
	defer c.Close()

	var scClient *sc.Client
	if scMCPURL != "" {
		scClient = sc.New(scMCPURL)
		log.Printf("worker | sc-local=%s", scMCPURL)
	}

	acts := &Activities{
		vault:        vault.New(vaultMCPURL),
		llm:          vcllm.New(litellmURL, litellmKey),
		sc:           scClient,
		defaultModel: defaultModel,
	}

	w := worker.New(c, TaskQueue, worker.Options{})
	w.RegisterWorkflow(ExecuteOpenSpecWorkflow)
	w.RegisterActivity(acts)

	log.Printf("worker | temporal=%s namespace=%s queue=%s vault=%s",
		temporalAddr, temporalNS, TaskQueue, vaultMCPURL)

	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalf("worker run: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
