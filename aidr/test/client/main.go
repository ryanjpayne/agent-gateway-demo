// Package main provides a gRPC test client for the AIDR ext_proc shim.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	address  = flag.String("address", "localhost:8080", "gRPC server address")
	useTLS   = flag.Bool("tls", false, "Use TLS for connection")
	payload  = flag.String("payload", "", "Path to JSON payload file")
	testdata = flag.String("testdata", "", "Path to testdata directory for batch mode")
	batch    = flag.Bool("batch", false, "Run all payloads in testdata directory")
	isResp   = flag.Bool("response", false, "Test as response body instead of request body")
	timeout  = flag.Duration("timeout", 30*time.Second, "Request timeout")
)

// Verdict represents the result of processing a payload.
type Verdict string

const (
	VerdictAllowed     Verdict = "ALLOWED"
	VerdictBlocked     Verdict = "BLOCKED"
	VerdictTransformed Verdict = "TRANSFORMED"
	VerdictError       Verdict = "ERROR"
)

// TestResult contains the result of testing a single payload.
type TestResult struct {
	PayloadFile     string
	Verdict         Verdict
	ResponseBody    string
	HTTPStatus      int
	TransformedBody json.RawMessage
	Error           error
}

func main() {
	flag.Parse()

	if *batch {
		if *testdata == "" {
			log.Fatal("--testdata is required when using --batch")
		}
		runBatchTests()
	} else {
		if *payload == "" {
			log.Fatal("--payload is required (or use --batch with --testdata)")
		}
		result := runSingleTest(*payload)
		printResult(result)
		if result.Verdict == VerdictError {
			os.Exit(1)
		}
	}
}

func runBatchTests() {
	files, err := filepath.Glob(filepath.Join(*testdata, "*.json"))
	if err != nil {
		log.Fatalf("Failed to glob testdata: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("No JSON files found in %s", *testdata)
	}

	fmt.Printf("\n=== Running batch tests from %s ===\n\n", *testdata)

	var results []TestResult
	for _, file := range files {
		result := runSingleTest(file)
		results = append(results, result)
		printResult(result)
		fmt.Println()
	}

	// Summary
	fmt.Println("=== Summary ===")
	allowed, blocked, transformed, errors := 0, 0, 0, 0
	for _, r := range results {
		switch r.Verdict {
		case VerdictAllowed:
			allowed++
		case VerdictBlocked:
			blocked++
		case VerdictTransformed:
			transformed++
		case VerdictError:
			errors++
		}
	}
	fmt.Printf("Total: %d | Allowed: %d | Blocked: %d | Transformed: %d | Errors: %d\n",
		len(results), allowed, blocked, transformed, errors)
}

func runSingleTest(payloadFile string) TestResult {
	result := TestResult{PayloadFile: payloadFile}

	// Read payload
	payloadBytes, err := os.ReadFile(payloadFile)
	if err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to read payload file: %w", err)
		return result
	}

	// Validate JSON
	var payload interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("invalid JSON in payload: %w", err)
		return result
	}

	// Connect to server
	conn, err := createConnection()
	if err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to connect: %w", err)
		return result
	}
	defer conn.Close()

	client := extprocv3.NewExternalProcessorClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Open bidirectional stream
	stream, err := client.Process(ctx)
	if err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to open stream: %w", err)
		return result
	}

	// Test flow depends on whether this is request or response
	if *isResp {
		return testResponseBody(stream, payloadBytes, result)
	}
	return testRequestBody(stream, payloadBytes, result)
}

func testRequestBody(stream extprocv3.ExternalProcessor_ProcessClient, payloadBytes []byte, result TestResult) TestResult {
	// Step 1: Send request headers
	if err := stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{},
		},
	}); err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to send request headers: %w", err)
		return result
	}

	// Receive headers response
	resp, err := stream.Recv()
	if err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to receive headers response: %w", err)
		return result
	}
	if _, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders); !ok {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("unexpected response type for headers: %T", resp.Response)
		return result
	}

	// Step 2: Send request body
	if err := stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{
				Body:        payloadBytes,
				EndOfStream: true,
			},
		},
	}); err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to send request body: %w", err)
		return result
	}

	// Receive body response
	resp, err = stream.Recv()
	if err != nil && err != io.EOF {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to receive body response: %w", err)
		return result
	}

	return interpretResponse(resp, result, true)
}

func testResponseBody(stream extprocv3.ExternalProcessor_ProcessClient, payloadBytes []byte, result TestResult) TestResult {
	// Step 1: Send response headers
	if err := stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_ResponseHeaders{
			ResponseHeaders: &extprocv3.HttpHeaders{},
		},
	}); err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to send response headers: %w", err)
		return result
	}

	// Receive headers response
	resp, err := stream.Recv()
	if err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to receive headers response: %w", err)
		return result
	}
	if _, ok := resp.Response.(*extprocv3.ProcessingResponse_ResponseHeaders); !ok {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("unexpected response type for headers: %T", resp.Response)
		return result
	}

	// Step 2: Send response body
	if err := stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_ResponseBody{
			ResponseBody: &extprocv3.HttpBody{
				Body:        payloadBytes,
				EndOfStream: true,
			},
		},
	}); err != nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to send response body: %w", err)
		return result
	}

	// Receive body response
	resp, err = stream.Recv()
	if err != nil && err != io.EOF {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("failed to receive body response: %w", err)
		return result
	}

	return interpretResponse(resp, result, false)
}

func interpretResponse(resp *extprocv3.ProcessingResponse, result TestResult, isRequest bool) TestResult {
	if resp == nil {
		result.Verdict = VerdictError
		result.Error = fmt.Errorf("nil response received")
		return result
	}

	// Check for immediate response (blocked)
	if ir, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse); ok {
		result.Verdict = VerdictBlocked
		if ir.ImmediateResponse.Status != nil {
			result.HTTPStatus = int(ir.ImmediateResponse.Status.Code)
		}
		if ir.ImmediateResponse.Body != nil {
			result.ResponseBody = string(ir.ImmediateResponse.Body)
		}
		return result
	}

	// Check for body mutation (transformed)
	var bodyResp *extprocv3.BodyResponse
	if isRequest {
		if br, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestBody); ok {
			bodyResp = br.RequestBody
		}
	} else {
		if br, ok := resp.Response.(*extprocv3.ProcessingResponse_ResponseBody); ok {
			bodyResp = br.ResponseBody
		}
	}

	if bodyResp != nil && bodyResp.Response != nil && bodyResp.Response.BodyMutation != nil {
		if bm, ok := bodyResp.Response.BodyMutation.Mutation.(*extprocv3.BodyMutation_Body); ok && bm.Body != nil {
			result.Verdict = VerdictTransformed
			result.TransformedBody = bm.Body
			return result
		}
	}

	result.Verdict = VerdictAllowed
	return result
}

func createConnection() (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	if *useTLS {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"h2"},
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return grpc.NewClient(*address, opts...)
}

func printResult(result TestResult) {
	filename := filepath.Base(result.PayloadFile)

	switch result.Verdict {
	case VerdictAllowed:
		fmt.Printf("[ALLOWED] %s\n", filename)
		fmt.Printf("  Payload passed through unchanged\n")

	case VerdictBlocked:
		fmt.Printf("[BLOCKED] %s\n", filename)
		if result.HTTPStatus > 0 {
			fmt.Printf("  HTTP Status: %d\n", result.HTTPStatus)
		}
		if result.ResponseBody != "" {
			fmt.Printf("  Response: %s\n", result.ResponseBody)
		}

	case VerdictTransformed:
		fmt.Printf("[TRANSFORMED] %s\n", filename)
		if result.TransformedBody != nil {
			prettyBytes, err := json.MarshalIndent(result.TransformedBody, "  ", "  ")
			if err == nil {
				lines := strings.Split(string(prettyBytes), "\n")
				// Show first 10 lines to avoid huge output
				maxLines := 10
				if len(lines) > maxLines {
					fmt.Printf("  Transformed body (first %d lines):\n", maxLines)
					for _, line := range lines[:maxLines] {
						fmt.Printf("  %s\n", line)
					}
					fmt.Printf("  ... (%d more lines)\n", len(lines)-maxLines)
				} else {
					fmt.Printf("  Transformed body:\n")
					for _, line := range lines {
						fmt.Printf("  %s\n", line)
					}
				}
			} else {
				fmt.Printf("  Transformed body: %s\n", string(result.TransformedBody))
			}
		}

	case VerdictError:
		fmt.Printf("[ERROR] %s\n", filename)
		fmt.Printf("  Error: %v\n", result.Error)
	}
}
