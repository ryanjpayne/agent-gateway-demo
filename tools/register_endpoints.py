import os
import subprocess
import sys
import json

def run_command(command, ignore_errors=False):
    """Run a shell command and print its output."""
    print(f"Running: {command}")
    result = subprocess.run(command, shell=True, text=True, capture_output=True)
    if result.returncode != 0 and not ignore_errors:
        print(f"Error executing: {command}")
        print(result.stderr)
        sys.exit(1)
    return result.stdout.strip()

def setup_agent_registry_consolidated():
    # 1. Resolve Environment Configuration
    project_id = os.getenv("PROJECT_ID")
    if not project_id:
        project_id = run_command("gcloud config get-value project", ignore_errors=True)
    if not project_id:
        print("Error: PROJECT_ID environment variable is required but not set, and could not be determined via gcloud config.")
        sys.exit(1)

    location = os.getenv("REGION")
    if not location:
        print("Error: REGION environment variable is required but not set.")
        sys.exit(1)

    print(f"Using Project ID: {project_id}")
    print(f"Using Location: {location}")

    # 13. Create Consolidated Agent Registry Service
    print("\n--- Creating Agent Registry Service ---")
    service_id = f"{location}-min-apis"
    service_check = run_command(f"gcloud alpha agent-registry services describe {service_id} --location={location} --project={project_id}", ignore_errors=True)
    
    if not service_check:
        print(f"Generating 4 endpoint flavors for each service...")
        
        target_services = [
            "agentregistry",
            "aiplatform",
            "cloudresourcemanager",
            "compute",
            "iap",
            "logging",
            "monitoring",
            "oauth2",
            "telemetry",
            "trace"
        ]
        
        urls = []
        for svc in target_services:
            # 1. Global
            urls.append(f"https://{svc}.googleapis.com")
            # 2. Global mTLS
            urls.append(f"https://{svc}.mtls.googleapis.com")
            # 3. Regional
            urls.append(f"https://{location}-{svc}.googleapis.com")
            # 4. Regional mTLS
            urls.append(f"https://{location}-{svc}.mtls.googleapis.com")

        # Format the interfaces block properly using HTTP_JSON binding
        interfaces = [{"url": url, "protocolBinding": "HTTP_JSON"} for url in urls]
        interfaces_json = json.dumps(interfaces)

        # Execute the consolidated registry registration
        run_command(
            f"gcloud alpha agent-registry services create {service_id} "
            f"--location={location} --project={project_id} "
            f'--display-name={service_id} '
            f'--description="Consolidated 4-flavor API endpoints to allow Agent Engine to function." '
            f"--endpoint-spec-type=no-spec "
            f"--interfaces='{interfaces_json}'"
        )
    else:
        print(f"Agent Registry service {service_id} already exists.")

    print("\nConsolidated Agent Registry Service step completed successfully.")

if __name__ == "__main__":
    setup_agent_registry_consolidated()
