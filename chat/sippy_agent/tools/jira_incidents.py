"""
Tool for querying Jira for known open incidents in the TRT project.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyJiraIncidentTool(SippyBaseTool):
    """Tool for querying Jira for known open incidents in the TRT project."""

    name: str = "check_known_incidents"
    description: str = "Get a JSON object with a list of all known open TRT incidents from Jira."

    # Add Jira configuration as proper fields
    jira_url: str = Field(default="https://issues.redhat.com", description="Jira instance URL")
    jira_username: Optional[str] = Field(default=None, description="Jira username")
    jira_token: Optional[str] = Field(default=None, description="Jira API token")

    class JiraIncidentInput(SippyToolInput):
        jira_url: Optional[str] = Field(default=None, description="Jira URL (optional, uses config if not provided)")

    args_schema: Type[SippyToolInput] = JiraIncidentInput

    def _run(self, jira_url: Optional[str] = None) -> Dict[str, Any]:
        """Query Jira for known open incidents."""
        # Use provided URL or fall back to instance URL
        api_url = jira_url or self.jira_url

        if not api_url:
            return {"error": "No Jira URL configured. Please set JIRA_URL environment variable or provide jira_url parameter."}

        # Construct the Jira REST API endpoint
        endpoint = f"{api_url.rstrip('/')}/rest/api/2/search"

        # Build JQL query for TRT project incidents
        jql_parts = ['project = "TRT"', 'labels = "trt-incident"', "status not in (Closed, Done, Resolved)"]

        jql = " AND ".join(jql_parts)

        try:
            # Prepare request parameters
            params = {
                "jql": jql,
                "fields": "key,summary,status,priority,created,updated,description,labels",
                "maxResults": 20,  # Limit results
            }

            # Prepare authentication if available
            auth = None
            if self.jira_username and self.jira_token:
                auth = (self.jira_username, self.jira_token)

            logger.info("Querying Jira for all open TRT incidents")
            logger.info(f"JQL: {jql}")

            # Make the API request
            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint, params=params, auth=auth, headers={"Accept": "application/json"})
                response.raise_for_status()

                data = response.json()

                # Add the user-friendly browse URL to each issue
                if "issues" in data and isinstance(data["issues"], list):
                    jira_base = api_url.rstrip("/")
                    for issue in data["issues"]:
                        if "key" in issue:
                            issue["browse_url"] = f"{jira_base}/browse/{issue['key']}"

                # Return the raw JSON data
                return data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error querying Jira: {e}")
            if e.response.status_code == 401:
                return {"error": "Jira authentication failed. Check JIRA_USERNAME and JIRA_TOKEN environment variables."}
            elif e.response.status_code == 403:
                return {"error": "Access denied to Jira. You may need authentication or permissions to view TRT project."}
            else:
                return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error querying Jira: {e}")
            return {"error": f"Failed to connect to Jira at {api_url} - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from Jira API"}
        except Exception as e:
            logger.error(f"Unexpected error querying Jira: {e}")
            return {"error": f"Unexpected error - {str(e)}"}
