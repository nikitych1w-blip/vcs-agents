// Generator for vcs-agents sub-configs.
//
// Usage:
//
//	go run . [ENV=local|prod]   generate configs (default ENV=prod)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const fileHeader = "# Generated from config.%s.yml — do not edit manually\n"

// ── config structs ────────────────────────────────────────────────────────────

type Config struct {
	DefaultModel string              `yaml:"default_model"`
	Models       map[string]Model    `yaml:"models"`
	Providers    map[string]Provider `yaml:"providers"`
	MCP          ConfigMCP           `yaml:"mcp"`
	LiteLLM      LiteLLMCfg         `yaml:"litellm"`
	OpenCode     OpenCodeCfg         `yaml:"opencode"`
	Auth2API     *Auth2APICfg        `yaml:"auth2api"` // nil in prod
	Worker       WorkerCfg           `yaml:"worker"`
}

type Model struct {
	Display  string `yaml:"display"`
	Provider string `yaml:"provider"`
	Upstream string `yaml:"upstream"`
}

type Provider struct {
	APIBaseEnv string `yaml:"api_base_env"`
	APIKeyEnv  string `yaml:"api_key_env"`
	APIKey     string `yaml:"api_key"`
}

type ConfigMCP map[string]MCPServer

type MCPServer struct {
	URLEnv        string `yaml:"url_env"`         // docker-internal URL (LiteLLM, workers)
	OpenCodeURLEnv string `yaml:"opencode_url_env"` // external URL for opencode (localhost)
	Transport     string `yaml:"transport"`
	AuthType      string `yaml:"auth_type"`
	TokenEnv      string `yaml:"token_env"`
}

type LiteLLMCfg struct {
	Router   map[string]any `yaml:"router"`
	Settings map[string]any `yaml:"settings"`
	General  struct {
		MasterKeyEnv   string `yaml:"master_key_env"`
		DatabaseURLEnv string `yaml:"database_url_env"`
		StoreModelInDB bool   `yaml:"store_model_in_db"`
	} `yaml:"general"`
	Observability struct {
		Langfuse struct {
			PublicKeyEnv string `yaml:"public_key_env"`
			SecretKeyEnv string `yaml:"secret_key_env"`
			HostEnv      string `yaml:"host_env"`
		} `yaml:"langfuse"`
	} `yaml:"observability"`
}

type OpenCodeCfg struct {
	LiteLLMBaseURLEnv string `yaml:"litellm_base_url_env"`
	LiteLLMAPIKeyEnv  string `yaml:"litellm_api_key_env"`
	LiteLLMMCPURLEnv  string `yaml:"litellm_mcp_url_env"`
}

type Auth2APICfg struct {
	Port     int    `yaml:"port"`
	APIKey   string `yaml:"api_key"`
	Timeouts struct {
		MessagesMS       int `yaml:"messages_ms"`
		StreamMessagesMS int `yaml:"stream_messages_ms"`
		CountTokensMS    int `yaml:"count_tokens_ms"`
	} `yaml:"timeouts"`
	Cloaking struct {
		CLIVersion string `yaml:"cli_version"`
		Entrypoint string `yaml:"entrypoint"`
	} `yaml:"cloaking"`
}

type WorkerCfg struct {
	TemporalNamespace string `yaml:"temporal_namespace"`
	VaultMCPURLEnv    string `yaml:"vault_mcp_url_env"`
}

// ── output structs ────────────────────────────────────────────────────────────

type liteLLMOut struct {
	ModelList       []liteLLMModel        `yaml:"model_list"`
	MCPServers      map[string]liteLLMMCP `yaml:"mcp_servers"`
	RouterSettings  map[string]any        `yaml:"router_settings"`
	LiteLLMSettings map[string]any        `yaml:"litellm_settings"`
	GeneralSettings liteLLMGeneral        `yaml:"general_settings"`
}

type liteLLMModel struct {
	ModelName     string        `yaml:"model_name"`
	LiteLLMParams liteLLMParams `yaml:"litellm_params"`
}

type liteLLMParams struct {
	Model   string `yaml:"model"`
	APIBase string `yaml:"api_base"`
	APIKey  string `yaml:"api_key"`
}

type liteLLMMCP struct {
	URL       string `yaml:"url"`
	Transport string `yaml:"transport"`
	AuthType  string `yaml:"auth_type"`
	Token     string `yaml:"token,omitempty"`
}

type liteLLMGeneral struct {
	MasterKey      string `yaml:"master_key"`
	DatabaseURL    string `yaml:"database_url"`
	StoreModelInDB bool   `yaml:"store_model_in_db"`
}

type auth2APIOut struct {
	Host     string           `yaml:"host"`
	Port     int              `yaml:"port"`
	AuthDir  string           `yaml:"auth-dir"`
	APIKeys  []string         `yaml:"api-keys"`
	Timeouts auth2APITimeouts `yaml:"timeouts"`
	Stats    map[string]bool  `yaml:"stats"`
	Debug    string           `yaml:"debug"`
	Cloaking auth2APICloaking `yaml:"cloaking"`
}

type auth2APITimeouts struct {
	MessagesMS       int `yaml:"messages-ms"`
	StreamMessagesMS int `yaml:"stream-messages-ms"`
	CountTokensMS    int `yaml:"count-tokens-ms"`
}

type auth2APICloaking struct {
	CLIVersion string `yaml:"cli-version"`
	Entrypoint string `yaml:"entrypoint"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

func osEnv(v string) string { return "os.environ/" + v }
func ocEnv(v string) string { return "{env:" + v + "}" }

// ── generators ────────────────────────────────────────────────────────────────

func genLiteLLM(cfg Config, env string) (string, error) {
	names := make([]string, 0, len(cfg.Models))
	for name := range cfg.Models {
		names = append(names, name)
	}
	sort.Strings(names)

	var models []liteLLMModel
	for _, name := range names {
		m := cfg.Models[name]
		prov := cfg.Providers[m.Provider]
		apiKey := prov.APIKey
		if prov.APIKeyEnv != "" {
			apiKey = osEnv(prov.APIKeyEnv)
		}
		models = append(models, liteLLMModel{
			ModelName: name,
			LiteLLMParams: liteLLMParams{
				Model:   m.Upstream,
				APIBase: osEnv(prov.APIBaseEnv),
				APIKey:  apiKey,
			},
		})
	}

	mcpServers := make(map[string]liteLLMMCP, len(cfg.MCP))
	mcpNames := make([]string, 0, len(cfg.MCP))
	for name := range cfg.MCP {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		srv := cfg.MCP[name]
		entry := liteLLMMCP{
			URL:       osEnv(srv.URLEnv),
			Transport: srv.Transport,
			AuthType:  srv.AuthType,
		}
		if srv.TokenEnv != "" {
			entry.Token = osEnv(srv.TokenEnv)
		}
		mcpServers[name+"_mcp"] = entry
	}

	g := cfg.LiteLLM.General
	out := liteLLMOut{
		ModelList:       models,
		MCPServers:      mcpServers,
		RouterSettings:  cfg.LiteLLM.Router,
		LiteLLMSettings: cfg.LiteLLM.Settings,
		GeneralSettings: liteLLMGeneral{
			MasterKey:      osEnv(g.MasterKeyEnv),
			DatabaseURL:    osEnv(g.DatabaseURLEnv),
			StoreModelInDB: g.StoreModelInDB,
		},
	}

	b, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(fileHeader, env) + string(b), nil
}

func genAuth2API(cfg Config, env string) (string, error) {
	a := cfg.Auth2API
	out := auth2APIOut{
		Host:    "0.0.0.0",
		Port:    a.Port,
		AuthDir: "/data",
		APIKeys: []string{a.APIKey},
		Timeouts: auth2APITimeouts{
			MessagesMS:       a.Timeouts.MessagesMS,
			StreamMessagesMS: a.Timeouts.StreamMessagesMS,
			CountTokensMS:    a.Timeouts.CountTokensMS,
		},
		Stats: map[string]bool{"enabled": true},
		Debug: "off",
		Cloaking: auth2APICloaking{
			CLIVersion: a.Cloaking.CLIVersion,
			Entrypoint: a.Cloaking.Entrypoint,
		},
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(fileHeader, env) + string(b), nil
}

func genOpenCode(cfg Config, env string) (string, error) {
	oc := cfg.OpenCode
	apiKeyEnv := oc.LiteLLMAPIKeyEnv

	names := make([]string, 0, len(cfg.Models))
	for name := range cfg.Models {
		names = append(names, name)
	}
	sort.Strings(names)

	models := make(map[string]map[string]string, len(names))
	for _, name := range names {
		models[name] = map[string]string{"name": cfg.Models[name].Display}
	}

	mcpBlock := map[string]any{
		"litellm": map[string]any{
			"type":    "remote",
			"url":     ocEnv(oc.LiteLLMMCPURLEnv),
			"enabled": true,
			"headers": map[string]string{
				"x-litellm-api-key": "Bearer " + ocEnv(apiKeyEnv),
			},
		},
	}
	// Add each MCP server that has an opencode_url_env configured.
	mcpNames := make([]string, 0, len(cfg.MCP))
	for name := range cfg.MCP {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		srv := cfg.MCP[name]
		if srv.OpenCodeURLEnv == "" {
			continue
		}
		entry := map[string]any{
			"type":    "remote",
			"url":     ocEnv(srv.OpenCodeURLEnv),
			"enabled": true,
		}
		mcpBlock[name] = entry
	}

	out := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"model":   "litellm/" + cfg.DefaultModel,
		"provider": map[string]any{
			"litellm": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "LiteLLM Gateway",
				"options": map[string]string{
					"baseURL": ocEnv(oc.LiteLLMBaseURLEnv),
					"apiKey":  ocEnv(apiKeyEnv),
				},
				"models": models,
			},
		},
		"mcp": mcpBlock,
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// genWorker produces configs/worker/config.generated.env with static (non-secret)
// worker settings. Secrets (TEMPORAL_ADDRESS, LITELLM_API_KEY, VAULT_MCP_URL, etc.)
// stay in .env.local and are injected via compose environment: block.
func genWorker(cfg Config, env string) (string, error) {
	ns := cfg.Worker.TemporalNamespace
	if ns == "" {
		ns = "default"
	}
	return fmt.Sprintf(fileHeader, env) +
		fmt.Sprintf("TEMPORAL_NAMESPACE=%s\n", ns) +
		fmt.Sprintf("DEFAULT_MODEL=%s\n", cfg.DefaultModel), nil
}

// ── orchestration ─────────────────────────────────────────────────────────────

type artifact struct {
	path    string
	content string
}

func build(cfg Config, env, root string) ([]artifact, error) {
	base := filepath.Join(root, "configs")

	ll, err := genLiteLLM(cfg, env)
	if err != nil {
		return nil, fmt.Errorf("litellm: %w", err)
	}
	oc, err := genOpenCode(cfg, env)
	if err != nil {
		return nil, fmt.Errorf("opencode: %w", err)
	}

	arts := []artifact{
		{filepath.Join(base, "litellm", "config.generated.yaml"), ll},
		{filepath.Join(root, "opencode.json"), oc},
	}

	if cfg.Auth2API != nil {
		a2, err := genAuth2API(cfg, env)
		if err != nil {
			return nil, fmt.Errorf("auth2api: %w", err)
		}
		arts = append(arts, artifact{filepath.Join(base, "auth2api", "config.generated.yaml"), a2})
	}

	if cfg.Worker.TemporalNamespace != "" {
		wk, err := genWorker(cfg, env)
		if err != nil {
			return nil, fmt.Errorf("worker: %w", err)
		}
		arts = append(arts, artifact{filepath.Join(base, "worker", "config.generated.env"), wk})
	}

	return arts, nil
}

func generate(cfg Config, env, root string) error {
	arts, err := build(cfg, env, root)
	if err != nil {
		return err
	}
	for _, a := range arts {
		if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(a.path, []byte(a.content), 0o644); err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, a.path)
		fmt.Println(" →", rel)
	}
	fmt.Printf("generated %d file(s)  (env=%s)\n", len(arts), env)
	return nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "config.prod.yml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("config.prod.yml not found in any parent directory")
		}
		dir = parent
	}
}

func main() {
	env := "prod"
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "ENV=") {
			env = strings.TrimPrefix(arg, "ENV=")
		}
	}

	root, err := findRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	cfgFile := filepath.Join(root, fmt.Sprintf("config.%s.yml", env))
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", cfgFile, err)
		os.Exit(1)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", cfgFile, err)
		os.Exit(1)
	}

	if err := generate(cfg, env, root); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
