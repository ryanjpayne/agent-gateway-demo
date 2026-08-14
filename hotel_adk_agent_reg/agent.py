import datetime
import os
from google.adk.agents import Agent
from google.adk.models import Gemini
from google.adk.tools.mcp_tool.mcp_toolset import MCPToolset, StreamableHTTPConnectionParams
from google.genai import types

LOCATION = os.environ.get("GOOGLE_CLOUD_LOCATION")
MCP_SERVER_URL = os.environ.get("MCP_SERVER_URL")
GOOGLE_CLOUD_PROJECT = os.environ.get("GOOGLE_CLOUD_PROJECT")

os.environ["GOOGLE_GENAI_USE_VERTEXAI"] = "True"

# Patch VertexAiSessionService.append_event to ensure event.timestamp is set.
# In google-adk==2.1.0, event.timestamp defaults to 0 (epoch), which the
# Vertex AI sessions API now rejects as "The timestamp of the event must be
# specified." Setting it to the current time when unset fixes this.
try:
    from google.adk.sessions.vertex_ai_session_service import VertexAiSessionService

    _original_append_event = VertexAiSessionService.append_event

    async def _patched_append_event(self, session, event):
        if not getattr(event, 'timestamp', None):
            try:
                object.__setattr__(
                    event,
                    'timestamp',
                    datetime.datetime.now(datetime.timezone.utc).timestamp(),
                )
            except Exception:
                pass
        return await _original_append_event(self, session=session, event=event)

    VertexAiSessionService.append_event = _patched_append_event
except Exception:
    pass

# Connect directly to the MCP server URL rather than looking it up via Agent
# Registry. The Agent Registry lookup (agentregistry.googleapis.com) fires at
# module import time and fails because the Agent Gateway intercepts that call
# with an SSL error before the container is fully initialised. Using the URL
# directly avoids that startup-time request while still routing all MCP tool
# calls through the Agent Gateway (the core of this demo).
mcp_toolset = MCPToolset(
    connection_params=StreamableHTTPConnectionParams(url=MCP_SERVER_URL)
)

root_agent = Agent(
        name="hotel_booking_agent_reg",
        description=(
            "You are a helpful hotel booking assistant. Use the provided tools to retrieve available hotels, book rooms, and check booking details. Always ask for clarification if the user does not provide enough details (such as dates, number of rooms, or guest name) for booking. Present the hotel list and booking details in a clean, professional, and readable markdown format."
        ),
        model=Gemini(
            model="gemini-2.5-flash",
            retry_options=types.HttpRetryOptions(attempts=3),
        ),
        tools=[mcp_toolset],
)
