package main

import (
	"encoding/json"
	"log"
	"net/http"
)

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

type Handler struct {
	repo  *RepoWriter
	tools []tool
}

func NewHandler(repo *RepoWriter) *Handler {
	return &Handler{
		repo: repo,
		tools: []tool{
			{
				Name:        "write_file",
				Description: "Write (create or overwrite) a file at the given path inside the local repo. Parent directories are created automatically.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"path": {
							Type:        "string",
							Description: "Relative file path inside the repo, e.g. 'src/api/routes.yaml'",
						},
						"content": {
							Type:        "string",
							Description: "Full text content to write to the file",
						},
					},
					Required: []string{"path", "content"},
				},
			},
			{
				Name:        "read_file",
				Description: "Read the contents of a file inside the local repo.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"path": {
							Type:        "string",
							Description: "Relative file path inside the repo",
						},
					},
					Required: []string{"path"},
				},
			},
			{
				Name:        "list_files",
				Description: "List files and directories at the given path inside the local repo. Defaults to the repo root.",
				InputSchema: inputSchema{
					Type: "object",
					Properties: map[string]property{
						"path": {
							Type:        "string",
							Description: "Relative directory path to list. Omit to list the repo root.",
						},
					},
					Required: []string{},
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
			ServerInfo:      serverInfo{Name: "vcs-sc-local-mcp", Version: "0.1.0"},
		})

	case "notifications/initialized":
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
	_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}
