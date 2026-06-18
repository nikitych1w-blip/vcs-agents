package sc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client calls the vcs-vcs-sc-local MCP server (or prod SourceControl API in the future).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

func (c *Client) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  toolCallParams{Name: name, Arguments: args},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sc http: %w", err)
	}
	defer resp.Body.Close()

	var rpc rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		return "", fmt.Errorf("sc decode: %w", err)
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("sc rpc %d: %s", rpc.Error.Code, rpc.Error.Message)
	}

	var result toolCallResult
	if err := json.Unmarshal(rpc.Result, &result); err != nil {
		return "", fmt.Errorf("sc result decode: %w", err)
	}
	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("sc tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("sc tool error")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("sc: empty response for %q", name)
	}
	return result.Content[0].Text, nil
}

func (c *Client) WriteFile(ctx context.Context, path, content string) (string, error) {
	return c.callTool(ctx, "write_file", map[string]any{"path": path, "content": content})
}

func (c *Client) ReadFile(ctx context.Context, path string) (string, error) {
	return c.callTool(ctx, "read_file", map[string]any{"path": path})
}

func (c *Client) ListFiles(ctx context.Context, path string) (string, error) {
	args := map[string]any{}
	if path != "" {
		args["path"] = path
	}
	return c.callTool(ctx, "list_files", args)
}
