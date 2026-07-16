# Agent Gateway + MCP + Custom Extension Demo

This repository has the following components:

1. An MCP server (`hotel_booker_direct_mcp`)
2. An ADK agent (`hotel_adk_agent_reg`) 
3. A Service Extension to be used to create a `CONTENT_AUTHZ` policy to intercept Agent to MCP calls. ( `debugger`)


> Note - This deployment setup is meant to be used till you complete the extension development, after that the extension should be deployed either as an authenticated Cloud Run Service or as a Backend Service in internal VPC (with Agent Gateway Network Attachment)

## Deployment Instructions 

Clone this repository to Google Cloud Shell.

```bash

cd agent-gateway-demo

uv sync

```

### Export variables and enable required APIs

```bash

export PROJECT_ID="<ADD GCP PROJECT ID HERE>"
# get the project number
export ORG_ID=<ORG_ID>
export PROJECT_NUMBER=$(gcloud projects describe ${PROJECT_ID} --format="value(projectNumber)")
export REGION="<ADD REGION HERE>"
export REPO_NAME=${REGION}-repo
export EXTENSION_NAME=cloudrun-extproc-extn
export ALL_AGENTS=principalSet://agents.global.org-${ORG_ID}.system.id.goog/attribute.platformContainer/aiplatform/projects/${PROJECT_NUMBER}
export PWD=`pwd`  

gcloud config set project $PROJECT_ID

```


Make sure you run all 3 blocks 

```bash

gcloud services enable run.googleapis.com \
      artifactregistry.googleapis.com \
      cloudbuild.googleapis.com

```


```bash

gcloud services enable \
  agentregistry.googleapis.com \
  aiplatform.googleapis.com \
  apphub.googleapis.com \
  apptopology.googleapis.com \
  cloudapiregistry.googleapis.com \
  cloudtrace.googleapis.com \
  compute.googleapis.com \
  dataform.googleapis.com \
  iam.googleapis.com \
  iamconnectors.googleapis.com \
  iap.googleapis.com \
  logging.googleapis.com \
  modelarmor.googleapis.com \
  monitoring.googleapis.com \
  networksecurity.googleapis.com \
  networkservices.googleapis.com \
  notebooks.googleapis.com \
  observability.googleapis.com


```

```bash

gcloud services enable \
  securitycenter.googleapis.com \
  saasservicemgmt.googleapis.com \
  storage.googleapis.com \
  telemetry.googleapis.com \
  texttospeech.googleapis.com \
  discoveryengine.googleapis.com

```

### MCP Server

The MCP server is deployed on Cloud run.

```bash



gcloud config set project $PROJECT_ID

gcloud run deploy hotel-booker-mcp-server \
    --source ./hotel_booker_direct_mcp \
    --region $REGION \
    --allow-unauthenticated \
    --set-env-vars MCP_TRANSPORT=streamable-http

```

Note down the Service URL which looks like https://hotel-booker-mcp-xxxxxx-uc.a.run.app



#### Add MCP server to the Registry

For this task we are going to use Google Cloud Console instead of `gcloud` commands


Search for Agent platform in Google Cloud Console. Click Agent Registry on the page, then MCP servers and then Add MCP Server - as shown below


![alt](./doc-images/mcp-1.png)


Fill in the details as shown below (use the region you have been using so far).

Copy the contents of `schema.json` from `./hotel_booker_direct_mcp` and add to `JSON` field in the screen (these are tools in our MCP server)

Click Next

> **IMPORTANT** When filling the MCP SERVER URL make sure you add `/mcp` at the end of the Cloud Run URL for the MCP server, Don't use Import Tools link.




![alt](./doc-images/mcp-2.png)

Click `Save` on the next screen.

![alt](./doc-images/mcp-3.png)

MCP server is added to the Agent Registry.
Note down the MCP SERVER NAME as shown in the diagram

![alt](./doc-images/mcp-4.png)




```bash

# e.g. agentregistry-00000000-0000-0000-2d19-a1259d005318
export MCP_SERVER_NAME=<ADD MCP_SERVER_NAME HERE>


```


### Agent Gateway

We are performing the following steps - 

1. Create Agent gateway
2. Deploy Service Extension Ext Proc Container
3. Create Service Extension
4. Create CONTENT_AUTHZ policy


#### 1. Create Agent gateway

For this task we are going to use Google Cloud Console instead of `gcloud` commands

Search for Agent Platform in Google Cloud Console, select Gateways and create the gateway as shown in the image below (make sure using the same region throughout)

![alt](./doc-images/agw-1.png)

#### 2. Deploy Service Extension Ext Proc Container

```bash


gcloud artifacts repositories create ${REPO_NAME} \
    --repository-format=docker \
    --location=${REGION} \
    --description="Docker repository for Service Extension Ext Proc"


docker build -t $REGION-docker.pkg.dev/$PROJECT_ID/$REPO_NAME/debugger:latest ./debugger


# deploy as cloud run service

gcloud run deploy debugger \
  --image={$REGION-docker.pkg.dev/$PROJECT_ID/$REPO_NAME/debugger:latest} \
  --region=${REGION} \
  --use-http2 \
  --port=50051 \
  --allow-unauthenticated

```

Note down the debugger service URL e.g. debugger-xxxxxx-uc.a.run.app as `EXTENSION_HOST`

```bash

export EXTENSION_HOST=<EXTENSION_HOST from above no URL scheme (https) prefix just the hostname>

```

#### 3. Create Service Extension


```bash



#Create the YAML file
cat <<EOF > cloudrun_debugger_extension.yaml
name: projects/${PROJECT_ID}/locations/${REGION}/authzExtensions/${EXTENSION_NAME}
service: ${EXTENSION_HOST}
authority: ${EXTENSION_HOST}
timeout: 10s
failOpen: true
description: "Public Cloud Run Debugger Extension"
EOF

# 3. Import the Service Extension
gcloud beta service-extensions authz-extensions import ${EXTENSION_NAME} \
  --source=cloudrun_debugger_extension.yaml \
  --location=${REGION} \
  --project=${PROJECT_ID}

```

#### 4. Create CONTENT_AUTHZ policy

```bash

cat <<EOF > authz-policy.yaml
name: projects/${PROJECT_ID}/locations/${REGION}/authzPolicies/${AUTHZ_POLICY_NAME}
action: CUSTOM
customProvider:
  authzExtension:
    resources:
    - projects/${PROJECT_NUMBER}/locations/${REGION}/authzExtensions/${EXTENSION_NAME}
policyProfile: CONTENT_AUTHZ
target:
  resources:
  - projects/${PROJECT_NUMBER}/locations/${REGION}/agentGateways/${AGENT_GATEWAY_NAME}
httpRules:
- to:
    operations:
    - paths:
      - prefix: /
  when: "request.host == '${EXTENSION_HOST}'"
EOF

gcloud beta network-security authz-policies import ${AUTHZ_POLICY_NAME} \
  --location=${REGION} \
  --source=authz-policy.yaml


```

### Agent

#### Locally Testing

Local testing does not engage the Agent Gateway, it is to verify the Agent and MCP functionality

```bash

cat <<EOF > hotel_adk_agent_reg/.env
GOOGLE_GENAI_USE_VERTEXAI=1
GOOGLE_CLOUD_PROJECT=${PROJECT_ID}
GOOGLE_CLOUD_LOCATION=${REGION}
MCP_SERVER_NAME=${MCP_SERVER_NAME}
EOF

```

```bash

uv run adk web

```

Access the Agent on local machine http://localhost:8000 / in Web preview on Cloud Shell (change port to 8000)

##### Sample prompts

1. Hello what can you do?
2. Please get details on booking HB-1


#### Provide IAM Access Agent Registry to all agents

```bash

gcloud projects add-iam-policy-binding ${PROJECT_ID} \
    --member="${ALL_AGENTS}" \
    --role="roles/agentregistry.viewer"

```

#### Deploy in Agent Runtime (Agent Engine)

We are now deploying the agent in agent engine, we first create a configuration to route the agent communication (EGRESS) via Agent gateway we created earlier

```bash

cat <<EOF > hotel_adk_agent_reg/agent_engine_config.json
{
  "agent_gateway_config": {
    "agent_to_anywhere_config": {
      "agent_gateway": "projects/${PROJECT_ID}/locations/${REGION}/agentGateways/${AGENT_GATEWAY_NAME}"
    }
  },
  "identity_type": "AGENT_IDENTITY",
  "env_vars": {
    "GOOGLE_API_PREVENT_AGENT_TOKEN_SHARING_FOR_GCP_SERVICES": "False"
  }
}
EOF


# Deploy agent in Agent Runtime (Agent Engine)

uv run adk deploy agent_engine --display_name hotel_agent --agent_engine_config_file ${PWD}/hotel_adk_agent_reg/agent_engine_config.json  hotel_adk_agent_reg/


```

