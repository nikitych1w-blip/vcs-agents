package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// ── JSON-RPC 2.0 types ─────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── MCP protocol types ──────────────────────────────────────────────────────

type initResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools *toolsCap `json:"tools,omitempty"`
}
type toolsCap struct{}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required"`
}

type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type toolsListResult struct {
	Tools []tool `json:"tools"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolCallResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ── Handler ─────────────────────────────────────────────────────────────────

type Handler struct {
	vault *VaultReader
	tools []tool
}

func NewHandler(vault *VaultReader) *Handler {
	return &Handler{
		vault: vault,
		tools: []tool{
			{
				Name:        "read_skill",
				Description: "Read a skill file from vcs-vault by skill ID. Returns the full markdown content.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"skill_id": {
							Type:        "string",
							Description: "Skill identifier, e.g. 'go-http-server' (maps to skills/go-http-server.md)",
						},
					},
					Required: []string{"skill_id"},
				},
			},
			{
				Name:        "list_openspec_changes",
				Description: "List all openspec changes in vcs-vault. Returns change IDs, their roles and available spec names.",
				InputSchema: inputSchema{
					Type:       "object",
					Properties: map[string]property{},
					Required:   []string{},
				},
			},
			{
				Name:        "read_openspec_proposal",
				Description: "Read the proposal (intent contract) for an openspec change. Contains business intent, success metrics, capabilities and impact.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"change_id": {
							Type:        "string",
							Description: "Change identifier, e.g. 'vcs-00000'",
						},
						"role": {
							Type:        "string",
							Description: "Role subdirectory, e.g. 'sa'. Omit to auto-detect.",
						},
					},
					Required: []string{"change_id"},
				},
			},
			{
				Name:        "read_openspec_spec",
				Description: "Read a specific spec file (requirements with SHALL statements and scenarios) for an openspec change.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"change_id": {
							Type:        "string",
							Description: "Change identifier, e.g. 'vcs-00000'",
						},
						"spec_name": {
							Type:        "string",
							Description: "Spec name, e.g. 'repos-search'",
						},
						"role": {
							Type:        "string",
							Description: "Role subdirectory, e.g. 'sa'. Omit to auto-detect.",
						},
					},
					Required: []string{"change_id", "spec_name"},
				},
			},
			{
				Name:        "search_knowledge",
				Description: "Search knowledge base files in vcs-vault for a given query string. Returns matching file paths and content snippets.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"query": {
							Type:        "string",
							Description: "Search query — matched against file names and content (case-insensitive)",
						},
					},
					Required: []string{"query"},
				},
			},
			{
				Name:        "read_schema",
				Description: "Read openspec schema.yaml from vcs-vault. Returns artifact definitions with id, generates, instruction and requires (DAG topology).",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"schema": {
							Type:        "string",
							Description: "Schema name, e.g. 'vcs'. Defaults to 'vcs' when omitted.",
						},
					},
					Required: []string{},
				},
			},
			{
				Name:        "read_step_prompt",
				Description: "Read the system prompt for a pipeline step from openspec/config.yaml. Joins shared context with step-specific role instructions.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"step_name": {
							Type:        "string",
							Description: "Step name as defined in pipeline.yaml, e.g. 'proposal', 'sa-specs', 'be-design'",
						},
					},
					Required: []string{"step_name"},
				},
			},
		},
	}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nil, -32700, "parse error")
		return
	}

	log.Printf("mcp | method=%s id=%v", req.Method, req.ID)

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "initialize":
		writeResult(w, req.ID, initResult{
			ProtocolVersion: "2025-03-26",
			Capabilities:    capabilities{Tools: &toolsCap{}},
			ServerInfo:      serverInfo{Name: "vcs-vault-local-mcp", Version: "0.1.0"},
		})

	case "notifications/initialized":
		// notification — no response
		w.WriteHeader(http.StatusAccepted)

	case "tools/list":
		writeResult(w, req.ID, toolsListResult{Tools: h.tools})

	case "tools/call":
		var p toolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			writeError(w, req.ID, -32602, "invalid params")
			return
		}
		result, err := h.callTool(r.Context(), p)
		if err != nil {
			writeResult(w, req.ID, toolCallResult{
				IsError: true,
				Content: []content{{Type: "text", Text: err.Error()}},
			})
			return
		}
		writeResult(w, req.ID, result)

	default:
		writeError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func writeResult(w http.ResponseWriter, id any, result any) {
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}
