// Package mcpapi implements the Atlas remote MCP endpoint over Streamable
// HTTP. It shares the REST application services: handlers only authenticate,
// validate transport input and map results.
package mcpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"atlas/internal/agentaccess"
	"atlas/internal/authorization"
	"atlas/internal/governance"
	"atlas/internal/identity"
	"atlas/internal/platform/apperr"
	"atlas/internal/platform/version"
	"atlas/internal/query"
)

const (
	defaultProtocolVersion = "2025-06-18"
	maxBodyBytes           = 1 << 20
)

var supportedProtocolVersions = map[string]struct{}{
	"2024-11-05": {},
	"2025-03-26": {},
	"2025-06-18": {},
}

// Deps are the services required by the MCP endpoint.
type Deps struct {
	Logger     *slog.Logger
	Agents     *agentaccess.Service
	Authorizer authorization.Service
	Query      *query.Service
	Governance *governance.Service
}

type handler struct {
	deps     Deps
	tools    []tool
	byName   map[string]tool
	sessions *sessionStore
}

// NewHandler builds the MCP HTTP handler.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	tools := newTools(deps)
	byName := make(map[string]tool, len(tools))
	for _, t := range tools {
		byName[t.name] = t
	}
	return &handler{deps: deps, tools: tools, byName: byName, sessions: newSessionStore()}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		h.terminateSession(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost+", "+http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
	}
	principal, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if sessionID := strings.TrimSpace(r.Header.Get(sessionIDHeader)); sessionID != "" {
		known, valid := h.sessions.validate(sessionID, principal)
		if !known {
			writeProblem(w, apperr.New(apperr.KindNotFound, "session_not_found", "MCP session not found."))
			return
		}
		if !valid {
			writeProblem(w, apperr.New(apperr.KindForbidden, "session_principal_mismatch", "MCP session belongs to another principal."))
			return
		}
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, nil, -32700, "parse error", nil)
		return
	}
	var request rpcRequest
	if err := json.Unmarshal(body, &request); err != nil {
		h.writeError(w, http.StatusBadRequest, nil, -32700, "parse error", nil)
		return
	}
	if request.Method == "" {
		h.writeError(w, http.StatusBadRequest, request.ID, -32600, "invalid request", nil)
		return
	}
	if header := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); header != "" {
		if _, ok := supportedProtocolVersions[header]; !ok {
			http.Error(w, "unsupported MCP protocol version", http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("MCP-Protocol-Version", defaultProtocolVersion)
	result, rpcErr := h.dispatch(r, principal, request)
	notification := len(bytes.TrimSpace(request.ID)) == 0
	if notification {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if rpcErr != nil {
		h.writeError(w, http.StatusOK, request.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
		return
	}
	if request.Method == "initialize" {
		w.Header().Set(sessionIDHeader, h.sessions.create(principal))
	}
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID, Result: result}
	if wantsEventStream(r) {
		encoded, err := json.Marshal(response)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, request.ID, -32603, "encode response", nil)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message\ndata: "))
		_, _ = w.Write(encoded)
		_, _ = w.Write([]byte("\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *handler) terminateSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get(sessionIDHeader))
	if sessionID == "" {
		writeProblem(w, apperr.New(apperr.KindInvalid, "session_required", "MCP-Session-Id is required."))
		return
	}
	known, valid := h.sessions.validate(sessionID, principal)
	if !known {
		writeProblem(w, apperr.New(apperr.KindNotFound, "session_not_found", "MCP session not found."))
		return
	}
	if !valid {
		writeProblem(w, apperr.New(apperr.KindForbidden, "session_principal_mismatch", "MCP session belongs to another principal."))
		return
	}
	h.sessions.delete(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func wantsEventStream(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
}

func (h *handler) authenticate(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	if h.deps.Agents == nil {
		h.writeError(w, http.StatusServiceUnavailable, nil, -32603, "agent authentication unavailable", nil)
		return identity.Principal{}, false
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := ""
	if len(header) >= 7 && strings.EqualFold(header[:7], "Bearer ") {
		token = strings.TrimSpace(header[7:])
	}
	principal, err := h.deps.Agents.AuthenticateToken(r.Context(), token)
	if err == nil {
		return principal, true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="atlas-mcp"`)
	writeProblem(w, err)
	return identity.Principal{}, false
}

func (h *handler) dispatch(r *http.Request, principal identity.Principal, request rpcRequest) (any, *rpcError) {
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		negotiated := defaultProtocolVersion
		if _, ok := supportedProtocolVersions[params.ProtocolVersion]; ok {
			negotiated = params.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": negotiated,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"listChanged": false, "subscribe": false},
			},
			"serverInfo": map[string]any{"name": "atlas-mcp", "version": version.Version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "notifications/initialized", "notifications/cancelled":
		return nil, nil
	case "tools/list":
		list := make([]map[string]any, 0, len(h.tools))
		for _, t := range h.tools {
			list = append(list, map[string]any{
				"name":        t.name,
				"description": t.description,
				"inputSchema": t.inputSchema,
			})
		}
		return map[string]any{"tools": list}, nil
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "resources/templates/list":
		return resourceTemplates(), nil
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.URI == "" {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		return h.readResource(r.Context(), principal, params.URI)
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		t, ok := h.byName[params.Name]
		if !ok {
			return nil, &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}
		}
		if err := h.deps.Authorizer.AuthorizeScope(principal, t.scope); err != nil {
			return nil, &rpcError{Code: -32003, Message: "insufficient scope for " + params.Name}
		}
		result, err := t.handler(r.Context(), principal, params.Arguments)
		if err != nil {
			return toolResult{
				Content: []toolContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			}, nil
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, &rpcError{Code: -32603, Message: "encode tool result"}
		}
		return toolResult{
			Content:           []toolContent{{Type: "text", Text: string(encoded)}},
			StructuredContent: result,
		}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + request.Method}
	}
}

func (h *handler) writeError(w http.ResponseWriter, status int, id json.RawMessage, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message, Data: data},
	})
}

// writeProblem returns an auth problem document without importing the HTTP API
// package (keeping MCP transport independent of the REST router).
func writeProblem(w http.ResponseWriter, err error) {
	problem := apperr.From(err)
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}
