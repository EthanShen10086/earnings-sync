package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/agent"
)

// Serve speaks MCP over stdio so Cursor or another host can let DeepSeek call the same tools.
func Serve(in io.Reader, out io.Writer, rt agent.Runtime) error {
	r := bufio.NewReader(in)
	for {
		body, err := readFrame(r)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &req) != nil || req.Method == "" || strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		result, rpcErr := dispatch(req.Method, req.Params, rt)
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		b, _ := json.Marshal(resp)
		if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(b), b); err != nil {
			return err
		}
	}
}

func dispatch(method string, params json.RawMessage, rt agent.Runtime) (any, map[string]any) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "earnings-sync", "version": "0.1.0"},
		}, nil
	case "tools/list":
		var tools []map[string]any
		for _, tool := range agent.Definitions() {
			tools = append(tools, map[string]any{
				"name": tool.Name, "description": tool.Description, "inputSchema": tool.Parameters,
			})
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(params, &call)
		raw, _ := json.Marshal(call.Arguments)
		text, _ := agent.Call(context.Background(), rt, call.Name, string(raw))
		return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}, nil
	default:
		return nil, map[string]any{"code": -32601, "message": "method not found"}
	}
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	var length int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			length, _ = strconv.Atoi(strings.TrimSpace(line[len("content-length:"):]))
		}
	}
	if length <= 0 {
		return nil, io.EOF
	}
	buf := make([]byte, length)
	_, err := io.ReadFull(r, buf)
	return buf, err
}
