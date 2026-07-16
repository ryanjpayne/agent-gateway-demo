import os
from google.adk.integrations.agent_registry import AgentRegistry
from google.auth import default
from google.adk.agents import Agent
from google.adk.models import Gemini
from google.genai import types

LOCATION = os.environ.get("GOOGLE_CLOUD_LOCATION")
MCP_SERVER_NAME = os.environ.get("MCP_SERVER_NAME")
GOOGLE_CLOUD_PROJECT = os.environ.get("GOOGLE_CLOUD_PROJECT")

os.environ["GOOGLE_GENAI_USE_VERTEXAI"] = "True"
registry = AgentRegistry(project_id=GOOGLE_CLOUD_PROJECT, location=LOCATION)

mcp_toolset = registry.get_mcp_toolset(
    f"projects/{GOOGLE_CLOUD_PROJECT}/locations/{LOCATION}/mcpServers/{MCP_SERVER_NAME}"
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