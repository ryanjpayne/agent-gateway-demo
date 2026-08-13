// Package server implements the Envoy ext_proc gRPC service for AIDR integration.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/crowdstrike/aidr-go"
	"github.com/crowdstrike/aidr-go/packages/param"
)

// maxBodySize is the maximum body size (10 MB) that the shim will attempt to
// parse and send to AIDR. Larger bodies are allowed through without scanning.
const maxBodySize = 10 * 1024 * 1024

// AIDRClient defines the interface for AIDR operations.
// This allows for mocking in tests.
type AIDRClient interface {
	GuardChatCompletions(ctx context.Context, params aidr.AIGuardGuardChatCompletionsParams) (*aidr.AIGuardGuardChatCompletionsResponse, error)
}

// aidrClientWrapper wraps the real AIDR client to implement AIDRClient.
type aidrClientWrapper struct {
	client *aidr.Client
}

func (w *aidrClientWrapper) GuardChatCompletions(ctx context.Context, params aidr.AIGuardGuardChatCompletionsParams) (*aidr.AIGuardGuardChatCompletionsResponse, error) {
	return w.client.AIGuard.GuardChatCompletions(ctx, params)
}

// NewAIDRClientWrapper creates a new wrapper around the AIDR client.
func NewAIDRClientWrapper(client *aidr.Client) AIDRClient {
	return &aidrClientWrapper{client: client}
}

// CalloutService implements the Envoy ExternalProcessor gRPC service.
type CalloutService struct {
	extprocv3.UnimplementedExternalProcessorServer
	aidrClient          AIDRClient
	collectorInstanceID string
	logger              *slog.Logger
	debugMode           bool
	echoMode            bool
	failClosed          bool
}

// CalloutServiceParams contains parameters for creating a CalloutService.
type CalloutServiceParams struct {
	AIDRClient          AIDRClient
	CollectorInstanceID string
	Logger              *slog.Logger
	DebugMode           bool
	EchoMode            bool
	FailClosed          bool
}

// NewCalloutService creates a new CalloutService.
func NewCalloutService(cs CalloutServiceParams) *CalloutService {
	return &CalloutService{
		aidrClient:          cs.AIDRClient,
		collectorInstanceID: cs.CollectorInstanceID,
		logger:              cs.Logger,
		debugMode:           cs.DebugMode,
		echoMode:            cs.EchoMode,
		failClosed:          cs.FailClosed,
	}
}

// Process implements the bidirectional streaming RPC for ext_proc.
func (s *CalloutService) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	ctx := stream.Context()
	requestID := generateRequestID()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// Context cancellation is normal when client disconnects
			if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
				s.logger.Debug("stream closed by client", "request_id", requestID)
				return nil
			}
			s.logger.Error("error receiving request", "error", err, "request_id", requestID)
			return status.Errorf(codes.Internal, "error receiving request: %v", err)
		}

		var resp *extprocv3.ProcessingResponse

		switch v := req.Request.(type) {
		case *extprocv3.ProcessingRequest_RequestHeaders:
			resp = s.handleRequestHeaders(ctx, v.RequestHeaders)
		case *extprocv3.ProcessingRequest_RequestBody:
			resp, err = s.handleRequestBody(ctx, v.RequestBody)
			if err != nil {
				s.logger.Error("processing request body failed",
					"error", err,
					"request_id", requestID,
				)
				resp = s.errorResponse(true)
			}
		case *extprocv3.ProcessingRequest_ResponseHeaders:
			resp = s.handleResponseHeaders(ctx, v.ResponseHeaders)
		case *extprocv3.ProcessingRequest_ResponseBody:
			resp, err = s.handleResponseBody(ctx, v.ResponseBody)
			if err != nil {
				s.logger.Error("processing response body failed",
					"error", err,
					"request_id", requestID,
				)
				resp = s.errorResponse(false)
			}
		default:
			resp = &extprocv3.ProcessingResponse{}
		}

		if err := stream.Send(resp); err != nil {
			s.logger.Error("error sending response", "error", err, "request_id", requestID)
			return status.Errorf(codes.Internal, "error sending response: %v", err)
		}
	}
}

func (s *CalloutService) handleRequestHeaders(_ context.Context, _ *extprocv3.HttpHeaders) *extprocv3.ProcessingResponse {
	// Continue processing headers, wait for body
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
	}
}

func (s *CalloutService) handleResponseHeaders(_ context.Context, _ *extprocv3.HttpHeaders) *extprocv3.ProcessingResponse {
	// Continue processing headers, wait for body
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseHeaders{
			ResponseHeaders: &extprocv3.HeadersResponse{},
		},
	}
}

func (s *CalloutService) handleRequestBody(ctx context.Context, body *extprocv3.HttpBody) (*extprocv3.ProcessingResponse, error) {
	return s.processBody(ctx, body.Body, aidr.AIGuardGuardChatCompletionsParamsEventTypeInput, true)
}

func (s *CalloutService) handleResponseBody(ctx context.Context, body *extprocv3.HttpBody) (*extprocv3.ProcessingResponse, error) {
	return s.processBody(ctx, body.Body, aidr.AIGuardGuardChatCompletionsParamsEventTypeOutput, false)
}

func (s *CalloutService) processBody(ctx context.Context, body []byte, eventType aidr.AIGuardGuardChatCompletionsParamsEventType, isRequest bool) (*extprocv3.ProcessingResponse, error) {
	eventTypeStr := "request"
	if !isRequest {
		eventTypeStr = "response"
	}

	s.logger.Debug("processing body",
		"type", eventTypeStr,
		"body_size", len(body),
	)

	// Empty bodies (e.g. chunked transfer or 204 responses) can't be scanned.
	if len(body) == 0 {
		return s.allowResponse(isRequest), nil
	}

	// Bodies exceeding maxBodySize are allowed through without scanning.
	if len(body) > maxBodySize {
		s.logger.Warn("body exceeds max size, skipping AIDR scan",
			"type", eventTypeStr,
			"body_size", len(body),
			"max_size", maxBodySize,
		)
		return s.allowResponse(isRequest), nil
	}

	// Parse the body as JSON to extract guard_input structure.
	// Non-JSON bodies (e.g. binary, form data) are allowed through without scanning.
	if !json.Valid(body) {
		s.logger.Debug("non-JSON body, skipping AIDR scan",
			"type", eventTypeStr,
			"body_size", len(body),
		)
		return s.allowResponse(isRequest), nil
	}

	var payload map[string]any
	// json.Valid guarantees well-formed JSON, so Unmarshal into map[string]any
	// only fails if the top-level value is not an object (e.g. array, string).
	// Those payloads have no extractable fields, so allow them through.
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal JSON body: %w", err)
	}

	// Echo mode: log and allow without calling AIDR
	if s.echoMode {
		s.logger.Info("echo mode: bypassing AIDR",
			"type", eventTypeStr,
			"body_size", len(body),
		)
		return s.allowResponse(isRequest), nil
	}

	// Build guard_input - the SDK accepts any as guard_input so we pass the parsed payload
	// which typically contains messages, tools, and other fields per the AI chat format.
	//
	// MCP protocol (primary):
	//   - Error responses (jsonrpc + error): allowed through without scanning.
	//   - Success responses (jsonrpc + result): scannable text extracted from result.
	//   - Requests (jsonrpc + method): scannable content extracted; unscannable methods
	//     (initialize, notifications) return nil to skip AIDR entirely.
	// OpenAI format (legacy fallback):
	//   - messages/tools extracted directly, or prompt converted to messages.
	var guardInput map[string]any
	if isMCPError(payload) {
		return s.allowResponse(isRequest), nil
	}
	if isMCPResponse(payload) {
		guardInput = s.extractMCPResponseContent(payload)
	} else {
		guardInput = s.buildGuardInput(payload)
	}
	if guardInput == nil {
		return s.allowResponse(isRequest), nil
	}

	// Call AIDR
	params := aidr.AIGuardGuardChatCompletionsParams{
		GuardInput: guardInput,
		EventType:  eventType,
	}

	if s.collectorInstanceID != "" {
		params.CollectorInstanceID = param.NewOpt(s.collectorInstanceID)
	}

	s.logger.Debug("calling AIDR API",
		"event_type", string(eventType),
	)

	aidrResp, err := s.aidrClient.GuardChatCompletions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("call AIDR API: %w", err)
	}

	s.logger.Debug("AIDR API response",
		"blocked", aidrResp.Result.Blocked,
		"transformed", aidrResp.Result.Transformed,
	)

	// Check for blocked content
	if aidrResp.Result.Blocked {
		s.logger.Info("request blocked by AIDR policy")
		resp, err := s.blockedResponse(isRequest)
		if err != nil {
			return nil, fmt.Errorf("create blocked response: %w", err)
		}
		return resp, nil
	}

	// Check for transformed content
	if aidrResp.Result.Transformed && aidrResp.Result.GuardOutput != nil {
		s.logger.Info("request transformed by AIDR policy")
		s.logger.Debug("transformed output", "has_guard_output", true)
		resp, err := s.transformedResponse(aidrResp.Result.GuardOutput, isRequest)
		if err != nil {
			return nil, fmt.Errorf("create transformed response: %w", err)
		}
		return resp, nil
	}

	// Allow unchanged
	return s.allowResponse(isRequest), nil
}

// buildGuardInput constructs the guard_input structure from the incoming payload.
// MCP JSON-RPC requests are the primary protocol: scannable content is extracted
// into messages format, and unscannable methods (initialize, notifications) return
// nil to signal that AIDR should be skipped. OpenAI Chat Completions format is
// supported as a legacy fallback.
func (s *CalloutService) buildGuardInput(payload map[string]any) map[string]any {
	// MCP JSON-RPC requests: extract scannable content or signal no-scan needed.
	if isMCPPayload(payload) {
		return s.extractMCPContent(payload) // nil means nothing to scan
	}

	// Legacy: OpenAI Chat Completions format
	guardInput := make(map[string]any)

	if s.extractMessagesAndTools(payload, guardInput) {
		return guardInput
	}

	if s.convertPromptToMessages(payload, guardInput) {
		return guardInput
	}

	return payload
}

// extractMessagesAndTools extracts messages and tools fields from the payload.
func (s *CalloutService) extractMessagesAndTools(payload, guardInput map[string]any) bool {
	hasContent := false
	if messages, ok := payload["messages"]; ok {
		guardInput["messages"] = messages
		hasContent = true
	}
	if tools, ok := payload["tools"]; ok {
		guardInput["tools"] = tools
		hasContent = true
	}
	return hasContent
}

// convertPromptToMessages converts a simple prompt field to messages format.
func (s *CalloutService) convertPromptToMessages(payload, guardInput map[string]any) bool {
	prompt, hasPrompt := payload["prompt"]
	if !hasPrompt {
		return false
	}

	if model, ok := payload["model"]; ok {
		guardInput["model"] = model
	}
	guardInput["messages"] = []map[string]any{
		{"role": "user", "content": prompt},
	}
	return true
}

// isMCPPayload detects whether the payload is an MCP JSON-RPC message by
// checking for the presence of "jsonrpc" and "method" fields.
func isMCPPayload(payload map[string]any) bool {
	_, hasJSONRPC := payload["jsonrpc"]
	_, hasMethod := payload["method"]
	return hasJSONRPC && hasMethod
}

// isMCPResponse detects whether the payload is an MCP JSON-RPC response
// (has "jsonrpc" and "result" but no "method").
func isMCPResponse(payload map[string]any) bool {
	_, hasJSONRPC := payload["jsonrpc"]
	_, hasResult := payload["result"]
	_, hasMethod := payload["method"]
	return hasJSONRPC && hasResult && !hasMethod
}

// isMCPError detects whether the payload is an MCP JSON-RPC error response
// (has "jsonrpc" and "error" but no "method"). These contain protocol-level
// errors, not user content, so they are allowed through without scanning.
func isMCPError(payload map[string]any) bool {
	_, hasJSONRPC := payload["jsonrpc"]
	_, hasError := payload["error"]
	_, hasMethod := payload["method"]
	return hasJSONRPC && hasError && !hasMethod
}

// extractMCPContent extracts scannable text from MCP JSON-RPC payloads and
// converts it into the OpenAI messages format that AIDR expects. Returns nil
// if no scannable content is found.
func (s *CalloutService) extractMCPContent(payload map[string]any) map[string]any {
	method, _ := payload["method"].(string)
	params, _ := payload["params"].(map[string]any)

	s.logger.Debug("detected MCP payload", "method", method)

	switch method {
	case "tools/call":
		return s.extractToolsCall(params)
	case "sampling/createMessage":
		return s.extractSamplingMessage(params)
	case "prompts/get":
		return s.extractPromptsGet(params)
	case "resources/read":
		return s.extractResourcesRead(params)
	default:
		// initialize, notifications, and other low-priority methods
		return nil
	}
}

// extractMCPResponseContent extracts scannable text from MCP JSON-RPC response
// payloads. Returns nil if no scannable content is found.
//
// Content sources are mapped to roles that reflect their origin:
//   - content[]/contents[]/structuredContent → role: "tool" (tool output)
//   - messages[] → role: "assistant" (model-generated)
//   - tools[] → role: "tool" (tool listing for poisoning detection)
func (s *CalloutService) extractMCPResponseContent(payload map[string]any) map[string]any {
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return nil
	}

	var toolTexts []string
	var assistantTexts []string

	// Extract from result.content[] and result.contents[]
	for _, key := range []string{"content", "contents"} {
		if items, ok := result[key].([]any); ok {
			for _, item := range items {
				if m, ok := item.(map[string]any); ok {
					if text, ok := m["text"].(string); ok {
						toolTexts = append(toolTexts, text)
					}
				}
			}
		}
	}

	// Extract from result.structuredContent (arbitrary JSON value).
	// Prevents bypass when a server returns only structuredContent with no content[].
	if sc, ok := result["structuredContent"]; ok {
		b, err := json.Marshal(sc)
		if err == nil {
			toolTexts = append(toolTexts, string(b))
		}
	}

	// Extract from result.tools[] (tools/list response — tool poisoning detection).
	// Serialize each tool's name, description, and inputSchema into scannable text.
	if tools, ok := result["tools"].([]any); ok {
		for _, item := range tools {
			t, ok := item.(map[string]any)
			if !ok {
				continue
			}
			b, err := json.Marshal(t)
			if err == nil {
				toolTexts = append(toolTexts, string(b))
			}
		}
	}

	// Extract from result.messages[] (sampling response).
	// Content may be a string, a single content block (map), or an array of
	// content blocks per the MCP sampling spec.
	if messages, ok := result["messages"].([]any); ok {
		for _, item := range messages {
			if m, ok := item.(map[string]any); ok {
				switch c := m["content"].(type) {
				case string:
					assistantTexts = append(assistantTexts, c)
				case map[string]any:
					if text, ok := c["text"].(string); ok {
						assistantTexts = append(assistantTexts, text)
					}
				case []any:
					for _, block := range c {
						if cb, ok := block.(map[string]any); ok {
							if text, ok := cb["text"].(string); ok {
								assistantTexts = append(assistantTexts, text)
							}
						}
					}
				}
			}
		}
	}

	if len(toolTexts) == 0 && len(assistantTexts) == 0 {
		return nil
	}

	var messages []map[string]any
	if len(toolTexts) > 0 {
		messages = append(messages, map[string]any{
			"role": "tool", "content": strings.Join(toolTexts, "\n"),
		})
	}
	if len(assistantTexts) > 0 {
		messages = append(messages, map[string]any{
			"role": "assistant", "content": strings.Join(assistantTexts, "\n"),
		})
	}

	return map[string]any{
		"messages": messages,
	}
}

// extractToolsCall extracts arguments from a tools/call request into messages.
// Arguments are serialized as JSON to preserve key names, nesting, and types,
// matching the proxy's JSON.stringify(args.params.arguments) behavior.
func (s *CalloutService) extractToolsCall(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}

	args, ok := params["arguments"].(map[string]any)
	if !ok || len(args) == 0 {
		return nil
	}

	content, err := json.Marshal(args)
	if err != nil {
		return nil
	}

	toolName, _ := params["name"].(string)

	messages := []map[string]any{
		{"role": "user", "content": string(content)},
	}

	guardInput := map[string]any{"messages": messages}
	if toolName != "" {
		guardInput["tools"] = []map[string]any{
			{"type": "function", "function": map[string]any{"name": toolName}},
		}
	}
	return guardInput
}

// extractSamplingMessage extracts messages and system prompt from a
// sampling/createMessage request.
func (s *CalloutService) extractSamplingMessage(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}

	guardInput := make(map[string]any)
	var messages []map[string]any

	// Add systemPrompt as a system message if present
	if sysPrompt, ok := params["systemPrompt"].(string); ok && sysPrompt != "" {
		messages = append(messages, map[string]any{
			"role": "system", "content": sysPrompt,
		})
	}

	// Extract messages array - MCP uses {role, content: {type, text}} format
	if rawMsgs, ok := params["messages"].([]any); ok {
		for _, raw := range rawMsgs {
			msg, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			if role == "" {
				role = "user"
			}

			var text string
			switch c := msg["content"].(type) {
			case string:
				text = c
			case map[string]any:
				text, _ = c["text"].(string)
			}

			if text != "" {
				messages = append(messages, map[string]any{
					"role": role, "content": text,
				})
			}
		}
	}

	if len(messages) == 0 {
		return nil
	}

	guardInput["messages"] = messages
	return guardInput
}

// extractPromptsGet extracts arguments from a prompts/get request.
func (s *CalloutService) extractPromptsGet(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}

	args, ok := params["arguments"].(map[string]any)
	if !ok || len(args) == 0 {
		return nil
	}

	var parts []string
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if v, ok := args[k].(string); ok {
			parts = append(parts, v)
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": strings.Join(parts, "\n")},
		},
	}
}

// extractResourcesRead extracts the URI from a resources/read request for
// path traversal scanning.
func (s *CalloutService) extractResourcesRead(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}

	uri, ok := params["uri"].(string)
	if !ok || uri == "" {
		return nil
	}

	return map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": uri},
		},
	}
}

// generateRequestID generates a random request ID using crypto/rand.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// errorResponse returns an allow or deny response based on the configured failure mode.
func (s *CalloutService) errorResponse(isRequest bool) *extprocv3.ProcessingResponse {
	if s.failClosed {
		resp, err := s.blockedResponse(isRequest)
		if err != nil {
			s.logger.Error("failed to create blocked response during error handling", "error", err)
			return s.allowResponse(isRequest)
		}
		return resp
	}
	return s.allowResponse(isRequest)
}

func (s *CalloutService) allowResponse(isRequest bool) *extprocv3.ProcessingResponse {
	if isRequest {
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{},
				},
			},
		}
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseBody{
			ResponseBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{},
			},
		},
	}
}

func (s *CalloutService) blockedResponse(isRequest bool) (*extprocv3.ProcessingResponse, error) {
	var blockMessage string
	if isRequest {
		blockMessage = "Request blocked by security policy"
	} else {
		blockMessage = "Response blocked by security policy"
	}

	bodyBytes, err := json.Marshal(map[string]string{
		"error": blockMessage,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal blocked response: %w", err)
	}

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode_Forbidden,
				},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: []*corev3.HeaderValueOption{
						{
							Header: &corev3.HeaderValue{
								Key:   "content-type",
								Value: "application/json",
							},
						},
					},
				},
				Body: bodyBytes,
			},
		},
	}, nil
}

// transformedResponse replaces the entire original body with AIDR's guard_output.
// This is intentional: AIDR returns the full sanitized payload, not a diff, so the
// original body must be fully replaced to apply the transformation.
func (s *CalloutService) transformedResponse(guardOutput any, isRequest bool) (*extprocv3.ProcessingResponse, error) {
	// Marshal the guard output back to JSON
	bodyBytes, err := json.Marshal(guardOutput)
	if err != nil {
		return nil, fmt.Errorf("marshal guard output: %w", err)
	}

	bodyMutation := &extprocv3.BodyMutation{
		Mutation: &extprocv3.BodyMutation_Body{
			Body: bodyBytes,
		},
	}

	// Update Content-Length to match the new body size, otherwise Envoy
	// forwards the original header which causes upstream errors.
	headerMutation := &extprocv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{
				Header: &corev3.HeaderValue{
					Key:   "content-length",
					Value: fmt.Sprintf("%d", len(bodyBytes)),
				},
			},
		},
	}

	if isRequest {
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						HeaderMutation: headerMutation,
						BodyMutation:   bodyMutation,
					},
				},
			},
		}, nil
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseBody{
			ResponseBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{
					HeaderMutation: headerMutation,
					BodyMutation:   bodyMutation,
				},
			},
		},
	}, nil
}
