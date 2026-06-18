package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

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
		return "", fmt.Errorf("vault http: %w", err)
	}
	defer resp.Body.Close()

	var rpc rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		return "", fmt.Errorf("vault decode: %w", err)
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("vault rpc %d: %s", rpc.Error.Code, rpc.Error.Message)
	}

	var result toolCallResult
	if err := json.Unmarshal(rpc.Result, &result); err != nil {
		return "", fmt.Errorf("vault result decode: %w", err)
	}
	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("vault tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("vault tool error")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("vault: empty response for %q", name)
	}
	return result.Content[0].Text, nil
}

func (c *Client) ListOpenSpecChanges(ctx context.Context) (string, error) {
	return c.callTool(ctx, "list_openspec_changes", map[string]any{})
}

func (c *Client) ReadOpenSpecProposal(ctx context.Context, changeID, role string) (string, error) {
	args := map[string]any{"change_id": changeID}
	if role != "" {
		args["role"] = role
	}
	return c.callTool(ctx, "read_openspec_proposal", args)
}

func (c *Client) ReadOpenSpecSpec(ctx context.Context, changeID, specName, role string) (string, error) {
	args := map[string]any{"change_id": changeID, "spec_name": specName}
	if role != "" {
		args["role"] = role
	}
	return c.callTool(ctx, "read_openspec_spec", args)
}

func (c *Client) ReadSkill(ctx context.Context, skillID string) (string, error) {
	return c.callTool(ctx, "read_skill", map[string]any{"skill_id": skillID})
}

func (c *Client) SearchKnowledge(ctx context.Context, query string) (string, error) {
	return c.callTool(ctx, "search_knowledge", map[string]any{"query": query})
}
