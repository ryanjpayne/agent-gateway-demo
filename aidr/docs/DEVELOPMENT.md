# Local Development

## Prerequisites

- Go 1.24+
- (Optional) Docker

## Running Tests

```bash
# Unit tests with race detector
go test -race ./...

# Unit tests with coverage
make test

# Lint
make lint
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make help` | Display all available targets |
| `make build` | Build the shim binary |
| `make run` | Run the shim locally |
| `make test` | Run tests with coverage |
| `make lint` | Run golangci-lint |
| `make lint-fix` | Run golangci-lint with auto-fix |
| `make fmt` | Run go fmt |
| `make vet` | Run go vet |
| `make clean` | Remove build artifacts |

## Running the Shim Locally

### Echo Mode (no AIDR credentials needed)

Echo mode bypasses the AIDR API entirely. The shim logs payloads and allows all requests through. Useful for testing the gRPC/ext_proc protocol without cloud access.

```bash
ECHO_MODE=true ALLOW_ECHO_MODE=true AIDR_CLOUD=us-1 AIDR_TOKEN=dummy LOG_LEVEL=debug make run
```

### Integration Testing (real AIDR credentials)

To test against the live AIDR API:

```bash
AIDR_CLOUD=us-1 AIDR_TOKEN=<your-token> LOG_LEVEL=debug DEBUG_MODE=true make run
```

`DEBUG_MODE=true` logs AIDR API request/response details.

## Using the Test Client

The test client in `test/client/` sends gRPC ext_proc requests to the running shim.

```bash
# Build the test client
go build -o testclient ./test/client
```

### Single Payload

```bash
./testclient --payload=test/testdata/clean_request.json
```

### Batch Mode (all payloads)

```bash
./testclient --batch --testdata=test/testdata/
```

### Test Response Bodies

By default the client sends request bodies. Use `--response` to test response-path processing:

```bash
./testclient --response --payload=test/testdata/response_with_pii.json
```

### Test Client Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--address` | `localhost:8080` | gRPC server address |
| `--tls` | `false` | Use TLS |
| `--payload` | | Path to a JSON payload file |
| `--testdata` | | Path to testdata directory (for `--batch`) |
| `--batch` | `false` | Run all JSON files in testdata directory |
| `--response` | `false` | Send as response body instead of request |
| `--timeout` | `30s` | Request timeout |

## Available Test Payloads

### OpenAI Format

| File | Description |
|------|-------------|
| `test/testdata/clean_request.json` | Normal request (should be allowed) |
| `test/testdata/pii_request.json` | Request containing PII |
| `test/testdata/prompt_injection.json` | Prompt injection attempt |
| `test/testdata/response_with_pii.json` | AI response containing PII |

### MCP JSON-RPC Format

| File | Description |
|------|-------------|
| `test/testdata/mcp_tools_call_injection.json` | SQL injection in tools/call arguments |
| `test/testdata/mcp_tools_call_pii.json` | PII (SSN, credit card) in tools/call arguments |
| `test/testdata/mcp_sampling_clean.json` | Clean sampling/createMessage request |
| `test/testdata/mcp_prompts_get.json` | prompts/get with template arguments |
| `test/testdata/mcp_resources_read.json` | Path traversal in resources/read URI |
| `test/testdata/mcp_response_pii.json` | PII in MCP tool response |
| `test/testdata/mcp_initialize.json` | Protocol handshake (bypasses AIDR) |
| `test/testdata/mcp_notification.json` | Cancellation notification (bypasses AIDR) |
| `test/testdata/mcp_error_response.json` | JSON-RPC error response (bypasses AIDR) |

## Local Envoy Integration Testing

A `docker-compose.yaml` and `envoy.yaml` are provided for end-to-end testing through a real Envoy proxy. This mimics how the shim runs in production (Envoy ext_proc filter → shim → upstream).

The stack runs three containers:

- **aidr-shim** — the ext_proc gRPC server (built from the repo Dockerfile)
- **envoy** — Envoy proxy with the ext_proc filter wired to the shim
- **echo-server** — a simple HTTP echo backend standing in for your upstream AI service

### Echo Mode (no credentials)

```bash
docker compose up --build
```

The default `docker-compose.yaml` starts the shim in echo mode (`ECHO_MODE=true`), which allows all requests through without calling AIDR. This is useful for testing the Envoy ↔ ext_proc integration itself.

Send a test request through Envoy:

```bash
curl -X POST http://localhost:10000/ \
  -H "Content-Type: application/json" \
  -d @test/testdata/mcp_tools_call_injection.json
```

### With Real AIDR Credentials

Override the environment variables to test against the live AIDR API:

```bash
ECHO_MODE=false AIDR_CLOUD=us-1 AIDR_TOKEN=<your-token> \
  docker compose up --build
```

### Test Harness Script

`scripts/test-local.sh` sends every fixture in `test/testdata/` through the running Envoy proxy and reports ALLOWED/BLOCKED for each:

```bash
# Run all fixtures
./scripts/test-local.sh

# Single fixture
./scripts/test-local.sh --single=test/testdata/mcp_tools_call_pii.json

# Verbose output
./scripts/test-local.sh --verbose
```

| Flag | Default | Description |
|------|---------|-------------|
| `--url=URL` | `http://localhost:10000` | Envoy listener URL |
| `--single=FILE` | | Test a single payload file |
| `--verbose` | | Show full curl headers and body |

### Ports

| Service | Port | Description |
|---------|------|-------------|
| Envoy listener | `10000` | Send HTTP requests here |
| Envoy admin | `9901` | Envoy admin dashboard |
| Shim gRPC | `8080` | Direct gRPC access (bypasses Envoy) |
| Shim health | `8081` | Health endpoint |
| Echo server | `8000` | Direct access to upstream echo backend |

### Tearing Down

```bash
docker compose down
```

## Health Checks

While the shim is running:

```bash
curl http://localhost:8081/health
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AIDR_CLOUD` | Yes | | Falcon cloud region (us-1, us-2, eu-1, us-gov-1, us-gov-2) |
| `AIDR_TOKEN` | Yes | | AIDR bearer token |
| `GRPC_PORT` | No | `8080` | gRPC server port |
| `HEALTH_PORT` | No | `8081` | Health check HTTP port |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |
| `DEBUG_MODE` | No | `false` | Verbose AIDR request/response logging |
| `ECHO_MODE` | No | `false` | Bypass AIDR API, allow all requests (requires `ALLOW_ECHO_MODE=true`) |
| `ALLOW_ECHO_MODE` | No | `false` | Safety guard for ECHO_MODE |
| `FAILURE_MODE` | No | `allow` | Behavior when AIDR is unreachable: `allow` or `deny` |
| `COLLECTOR_INSTANCE_ID` | No | | Optional instance identifier |

## Deployment Scripts

### Debug Deployment

Deploy with debug logging enabled:

```bash
./scripts/deploy.sh --debug
```

### Cloud Run Smoke Tests

After deploying, verify the service is healthy:

```bash
./scripts/test-cloud-run.sh
```

### Cleanup

Remove all deployed resources (Cloud Run service, secrets, container images):

```bash
./scripts/cleanup.sh
```

## Docker

```bash
# Build
docker build -t aidr-shim:local .

# Run with echo mode
docker run -p 8080:8080 -p 8081:8081 \
  -e ECHO_MODE=true -e ALLOW_ECHO_MODE=true -e AIDR_CLOUD=us-1 -e AIDR_TOKEN=dummy \
  aidr-shim:local

# Run with real credentials
docker run -p 8080:8080 -p 8081:8081 \
  -e AIDR_CLOUD=us-1 -e AIDR_TOKEN=<your-token> \
  aidr-shim:local
```
