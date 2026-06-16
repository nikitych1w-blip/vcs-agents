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
				Description: "Read a skill file from vcs-vcs-vault-local by skill ID. Returns the full markdown content of the skill.",
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
				Name:        "read_openspec",
				Description: "Read an OpenSpec task definition from vcs-vcs-vault-local by spec ID. Returns YAML content describing the task, pipeline, and context.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"spec_id": {
							Type:        "string",
							Description: "Spec identifier, e.g. 'feat-123' (maps to openspecs/feat-123.yaml)",
						},
					},
					Required: []string{"spec_id"},
				},
			},
			{
				Name:        "search_knowledge",
				Description: "Search knowledge base files in vcs-vcs-vault-local for a given query string. Returns list of matching file paths and their content snippets.",
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
