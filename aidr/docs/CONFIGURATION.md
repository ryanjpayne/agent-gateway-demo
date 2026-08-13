# Configuration Reference

Complete reference for all configuration options of the AIDR GCP ext_proc shim.

## Environment Variables

The shim is configured entirely through environment variables. In production, sensitive values should be stored in Google Secret Manager.

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `AIDR_CLOUD` | CrowdStrike Falcon cloud region | `us-1` |
| `AIDR_TOKEN` | Bearer token for AIDR API authentication | `eyJhbGciOiJIUzI1NiIs...` |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GRPC_PORT` | `8080` | Port for the gRPC ext_proc server |
| `HEALTH_PORT` | `8081` | Port for the HTTP health check endpoint |
| `LOG_LEVEL` | `info` | Logging verbosity level |
| `DEBUG_MODE` | `false` | Enable verbose debug logging |
| `ECHO_MODE` | `false` | Bypass AIDR, log payloads only (testing) |
| `FAILURE_MODE` | `allow` | Behavior when AIDR is unreachable: `allow` or `deny` |
| `ALLOW_ECHO_MODE` | `false` | Safety guard: must be `true` to enable `ECHO_MODE` |
| `COLLECTOR_INSTANCE_ID` | (empty) | Optional identifier for this shim instance |

---

## Variable Details

### AIDR_CLOUD

**Required**: Yes
**Type**: String (cloud region identifier)
**Secret**: No

The CrowdStrike Falcon cloud region. The shim derives the full AIDR API base URL automatically from this value.

**Valid values**: `us-1`, `us-2`, `eu-1`, `us-gov-1`, `us-gov-2`

**Example**:

```bash
AIDR_CLOUD="us-1"
```

### AIDR_TOKEN

**Required**: Yes
**Type**: String (Bearer Token)
**Secret**: Yes (store in Secret Manager)

The bearer token for authenticating with the AIDR API. Generate this in the CrowdStrike Falcon console.

**Security**: Never commit this value to version control. Always use Secret Manager in production.

**Example**:

```bash
AIDR_TOKEN="your-bearer-token-here"
```

### GRPC_PORT

**Required**: No
**Type**: Integer
**Default**: `8080`

The port on which the gRPC ext_proc server listens. Cloud Run routes traffic to the `PORT` environment variable (which defaults to 8080), so this should typically remain at the default.

**When to change**: Only if you're running multiple services in the same container or have specific port requirements.

### HEALTH_PORT

**Required**: No
**Type**: Integer
**Default**: `8081`

The port for the HTTP health check server. This exposes:

- `GET /health` - Returns `OK` when the service is running

**Note**: Cloud Run uses TCP health checks by default, but you can configure HTTP health checks to use this endpoint.

### LOG_LEVEL

**Required**: No
**Type**: String
**Default**: `info`
**Values**: `debug`, `info`, `warn`, `error`

Controls the verbosity of log output.

| Level | Description | Use Case |
|-------|-------------|----------|
| `debug` | All messages including internal details | Development, troubleshooting |
| `info` | Normal operational messages | Production (default) |
| `warn` | Warnings and potential issues | Production with reduced noise |
| `error` | Errors only | Minimal logging |

**Example**:

```bash
LOG_LEVEL="debug"  # For troubleshooting
LOG_LEVEL="warn"   # For high-volume production
```

### DEBUG_MODE

**Required**: No
**Type**: Boolean
**Default**: `false`
**Values**: `true`, `false`

Enables verbose debug logging including full request and response bodies.

**Warning**: This may log sensitive data (PII, prompts, API responses). Use only for troubleshooting and disable in production.

**Example**:

```bash
DEBUG_MODE="true"  # Temporary troubleshooting
DEBUG_MODE="false" # Normal operation
```

### ECHO_MODE

**Required**: No
**Type**: Boolean
**Default**: `false`
**Values**: `true`, `false`

When enabled, the shim bypasses the AIDR API entirely and:

- Logs all request/response payloads
- Always returns "allow" decisions

**Use case**: Testing the ext_proc integration without AIDR connectivity.

**Warning**: Never enable in production - all requests will be allowed without inspection.

**Safety guard**: `ECHO_MODE=true` requires `ALLOW_ECHO_MODE=true` to be set as well. This prevents accidental activation in production.

### ALLOW_ECHO_MODE

**Required**: No
**Type**: Boolean
**Default**: `false`
**Values**: `true`, `false`

Safety guard for `ECHO_MODE`. Must be explicitly set to `true` before `ECHO_MODE` can be enabled. This is a deliberate two-flag mechanism to prevent accidental bypass of AIDR scanning in production.

### FAILURE_MODE

**Required**: No
**Type**: String
**Default**: `allow`
**Values**: `allow`, `deny`

Controls behavior when the AIDR API is unreachable or returns an error:

| Value | Behavior | Use Case |
|-------|----------|----------|
| `allow` | Requests pass through without scanning | Availability-first environments |
| `deny` | Requests are blocked | Security-first environments |

**Example**:

```bash
FAILURE_MODE="deny"  # Block traffic when AIDR is down
FAILURE_MODE="allow" # Allow traffic when AIDR is down (default)
```

### COLLECTOR_INSTANCE_ID

**Required**: No
**Type**: String
**Default**: (empty)

An optional identifier for this shim instance. Included in logs and AIDR API calls for correlation.

**Use cases**:

- Identifying traffic sources in multi-region deployments
- Correlating logs with specific Cloud Run revisions
- Debugging multi-instance scenarios

**Example**:

```bash
COLLECTOR_INSTANCE_ID="prod-us-central1-v2"
```

---

## Secret Manager Configuration

In production, store sensitive values in Google Secret Manager.

### Creating Secrets

```bash
# Create AIDR cloud secret
echo -n "us-1" | \
  gcloud secrets create aidr-cloud --data-file=-

# Create AIDR token secret
echo -n "your-bearer-token" | \
  gcloud secrets create aidr-token --data-file=-
```

### Updating Secrets

```bash
# Add new version (previous versions remain accessible)
echo -n "new-value" | \
  gcloud secrets versions add aidr-cloud --data-file=-
```

### Cloud Run Secret Mounting

When deploying with the scripts or Terraform, secrets are automatically mounted:

```bash
gcloud run deploy aidr-shim \
  --set-secrets="AIDR_CLOUD=aidr-cloud:latest,AIDR_TOKEN=aidr-token:latest"
```

---

## Resource Sizing

Recommended resource configurations based on traffic volume:

### Development/Testing

```yaml
cpu: 1
memory: 256Mi
min_instances: 0
max_instances: 2
```

### Low-Volume Production

```yaml
cpu: 1
memory: 512Mi
min_instances: 0
max_instances: 10
```

### Medium-Volume Production

```yaml
cpu: 1
memory: 512Mi
min_instances: 1      # Keep warm
max_instances: 50
```

### High-Volume Production

```yaml
cpu: 2
memory: 1Gi
min_instances: 2      # Redundancy
max_instances: 100
```

---

## Cloud Run Settings

### Required Settings

| Setting | Value | Reason |
|---------|-------|--------|
| `--use-http2` | Required | ext_proc uses gRPC (HTTP/2) |
| `--port=8080` | Default | Must match GRPC_PORT |
| `--allow-unauthenticated` | Typical | LB needs access |

### Recommended Settings

| Setting | Recommended | Description |
|---------|-------------|-------------|
| `--cpu=1` to `--cpu=2` | Based on load | Start with 1, increase if needed |
| `--memory=512Mi` to `--memory=1Gi` | Based on payload size | Larger for big AI responses |
| `--concurrency=80` | Default (80) | Concurrent requests per instance |
| `--timeout=300s` | Default | Request timeout |

---

## Example Configurations

### Minimal (Development)

```bash
# Environment variables
export AIDR_CLOUD="us-1"
export AIDR_TOKEN="dev-token"
export DEBUG_MODE="true"
export LOG_LEVEL="debug"
```

### Production

```bash
# Deployment command
gcloud run deploy aidr-shim \
  --region=us-central1 \
  --set-env-vars="LOG_LEVEL=info" \
  --set-secrets="AIDR_CLOUD=aidr-cloud:latest,AIDR_TOKEN=aidr-token:latest" \
  --cpu=2 \
  --memory=1Gi \
  --min-instances=1 \
  --max-instances=100 \
  --use-http2
```

### Terraform (terraform.tfvars)

```hcl
project_id    = "my-project"
region        = "us-central1"
service_name  = "aidr-shim"

# Secrets (use TF_VAR_ environment variables for CI/CD)
aidr_cloud    = "us-1"
aidr_token    = "prod-token"

# Resources
cpu           = "2"
memory        = "1Gi"
min_instances = 1
max_instances = 100

# Logging
log_level  = "info"
debug_mode = false

# Instance ID for multi-region
collector_instance_id = "prod-us-central1"
```

---

## Validation

The shim validates configuration at startup. Invalid configuration causes the service to exit with an error.

### Common Validation Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `AIDR_CLOUD environment variable is required` | Missing cloud region | Set AIDR_CLOUD or mount secret |
| `AIDR_TOKEN environment variable is required` | Missing or whitespace-only token | Set AIDR_TOKEN or mount secret |
| `invalid GRPC_PORT` | Non-numeric or out-of-range port | Use integer value between 1 and 65535 |
| `invalid HEALTH_PORT` | Non-numeric or out-of-range port | Use integer value between 1 and 65535 |
| `invalid LOG_LEVEL` | Unrecognized log level | Use one of: debug, info, warn, error |
| `invalid FAILURE_MODE` | Unrecognized failure mode | Use `allow` or `deny` |
| `ECHO_MODE=true requires ALLOW_ECHO_MODE=true` | Echo mode safety guard | Set `ALLOW_ECHO_MODE=true` or disable `ECHO_MODE` |

### Startup Logging

On successful startup, the shim logs its configuration as structured JSON (via `slog`):

```json
{"level":"INFO","msg":"starting AIDR ext_proc shim","mode":"normal","debug_mode":false,"echo_mode":false}
{"level":"INFO","msg":"starting gRPC server","port":8080}
{"level":"INFO","msg":"starting health HTTP server","port":8081}
```
