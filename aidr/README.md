# AIDR GCP ext_proc Shim

A gRPC service that integrates
[CrowdStrike AIDR](https://www.crowdstrike.com/) (AI Detection & Response)
with Google Cloud Load Balancer's
[external processing](https://cloud.google.com/load-balancing/docs/https/ext-proc-overview)
(ext_proc) extension.

## Overview

This shim enables real-time protection for AI workloads by:

- **Detecting prompt injection attacks** before they reach your AI systems
- **Preventing data leakage** by identifying PII and sensitive data in requests/responses
- **Enforcing security policies** defined in CrowdStrike Falcon

```mermaid
flowchart LR
    User([User]) -->|Request| LB[Cloud Load\nBalancer]
    LB -->|ext_proc\ngRPC| Shim[AIDR\next_proc Shim]
    Shim -->|Analyze| AIDR[CrowdStrike\nAIDR API]
    AIDR -->|Verdict| Shim
    Shim -->|Allow / Block\nTransform| LB
    LB -->|Forward| App[Your AI App]
```

## Quick Start

### Prerequisites

- GCP project with billing enabled
- CrowdStrike AIDR credentials (cloud region and bearer token)
- [gcloud CLI](https://cloud.google.com/sdk/docs/install) or Google Cloud Shell

### Deploy in 2 Minutes

```bash
# 1. Clone the repository
git clone <repository-url>
cd aidr-shim

# 2. Set up prerequisites
./scripts/setup-prerequisites.sh

# 3. Deploy (interactive prompts for configuration)
./scripts/deploy.sh
```

The deployment script will:

1. Validate your GCP environment
2. Create secrets in Secret Manager
3. Deploy to Cloud Run
4. Output the service URL for Load Balancer configuration

### Non-Interactive Deployment

```bash
export PROJECT_ID="your-project-id"
export AIDR_CLOUD="us-1"
export AIDR_TOKEN="your-bearer-token"

./scripts/deploy.sh
```

## Documentation

| Document | Description |
|----------|-------------|
| [User Guide](docs/USER_GUIDE.md) | Complete deployment and usage guide |
| [Configuration](docs/CONFIGURATION.md) | All configuration options |
| [Development](docs/DEVELOPMENT.md) | Local development guide |
| [Terraform](terraform/README.md) | Infrastructure-as-code deployment |

## Deployment Options

### Option 1: Shell Scripts (Quick Start)

Best for getting started quickly in Cloud Shell:

```bash
./scripts/deploy.sh
```

### Option 2: Terraform (Production)

Best for repeatable, auditable deployments:

```bash
cd terraform
terraform init
terraform apply
```

See [terraform/README.md](terraform/README.md) for details.

## Testing

Test your deployment with the included test client:

```bash
# Build test client
go build -o testclient ./test/client

# Test a payload
./testclient \
  --address=YOUR_SERVICE_URL:443 \
  --tls \
  --payload=test/testdata/clean_request.json

# Run all test payloads
./testclient \
  --address=YOUR_SERVICE_URL:443 \
  --tls \
  --batch \
  --testdata=test/testdata
```

## Project Structure

```text
├── cmd/
│   └── shim/           # Main service
├── internal/
│   ├── config/         # Configuration
│   └── server/         # gRPC server implementation
├── scripts/
│   ├── deploy.sh           # Production deployment (--debug flag for debug mode)
│   ├── setup-prerequisites.sh
│   ├── test-cloud-run.sh   # Cloud Run smoke tests
│   ├── test-local.sh       # Local Envoy integration tests
│   └── cleanup.sh          # Resource cleanup
├── terraform/          # Terraform module
├── test/
│   ├── client/        # Test client
│   └── testdata/      # Test payloads
├── docs/               # Documentation
├── Dockerfile
└── README.md
```

## Configuration

Key environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `AIDR_CLOUD` | Yes | Falcon cloud region (e.g. us-1, us-2, eu-1) |
| `AIDR_TOKEN` | Yes | Bearer token for authentication |
| `LOG_LEVEL` | No | debug, info, warn, error (default: info) |
| `DEBUG_MODE` | No | Enable verbose logging (default: false) |
| `FAILURE_MODE` | No | `allow` (default) or `deny` — controls behavior when AIDR is unreachable |
| `ECHO_MODE` | No | Bypass AIDR scanning (requires `ALLOW_ECHO_MODE=true`) |

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for all options.

## Next Steps After Deployment

1. **Configure Load Balancer**: Provide the service URL to Google for ext_proc configuration
2. **Set Up Policies**: Configure AIDR policies in CrowdStrike Falcon console
3. **Monitor**: View logs with `gcloud run services logs read aidr-shim --limit=50`

## License

Copyright CrowdStrike. See LICENSE for details.
