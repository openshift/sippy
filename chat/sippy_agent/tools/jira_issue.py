"""
Tool for analyzing Jira issues and their comments.
"""

import base64
import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx
from datetime import datetime, timezone

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyJiraIssueTool(SippyBaseTool):
    """Tool for analyzing Jira issues and their comments to assess fix readiness."""

    name: str = "get_jira_issue_analysis"
    description: str = """Get Jira issue information including description, status, and recent comments.

This tool provides:
- Issue description and current status
- Recent comments (sorted newest first)
- Basic metadata (assignee, priority, fix versions, etc.)

Input: issue_key (the Jira issue key, e.g., OCPBUGS-12345)"""

    jira_url: str = Field(description="Jira base URL")
    jira_basic_auth_token: Optional[str] = Field(default=None, description="Jira basic auth token (user:api_token)")

    class JiraIssueInput(SippyToolInput):
        issue_key: str = Field(description="Jira issue key (e.g., OCPBUGS-12345)")

    args_schema: Type[SippyToolInput] = JiraIssueInput

    def _get_auth_headers(self) -> Dict[str, str]:
        """Build authentication headers for Jira API requests."""
        headers = {"Accept": "application/json"}
        if self.jira_basic_auth_token:
            encoded = base64.b64encode(self.jira_basic_auth_token.encode()).decode()
            headers["Authorization"] = f"Basic {encoded}"
        return headers

    def _run(self, *args, **kwargs: Any) -> Dict[str, Any]:
        """Get Jira issue analysis."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        # Pydantic model will have validated and filled in defaults
        args = self.JiraIssueInput(**input_data)

        issue_key = args.issue_key.strip()

        try:
            # Make API request to get issue details using v3 API
            api_url = f"{self.jira_url.rstrip('/')}/rest/api/3/issue/{issue_key}"

            logger.info(f"Making request to {api_url}")

            headers = self._get_auth_headers()

            with httpx.Client(timeout=30.0) as client:
                # Get issue details
                response = client.get(api_url, headers=headers)
                response.raise_for_status()

                issue_data = response.json()

                # Get comments
                comments_response = client.get(f"{api_url}/comment", headers=headers)
                comments_response.raise_for_status()
                comments_data = comments_response.json()

                # Process the response
                processed_data = self._process_jira_issue(issue_data, comments_data)

                return processed_data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting Jira issue: {e}")
            if e.response.status_code == 404:
                return {"error": f"Jira issue {issue_key} not found or access denied"}
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error getting Jira issue: {e}")
            return {"error": f"Failed to connect to Jira at {self.jira_url} - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from Jira API"}
        except Exception as e:
            logger.error(f"Unexpected error getting Jira issue: {e}")
            return {"error": f"Unexpected error - {str(e)}"}

    def _process_jira_issue(self, issue_data: Dict[str, Any], comments_data: Dict[str, Any]) -> Dict[str, Any]:
        """Process Jira issue data and comments to extract key information."""

        fields = issue_data.get('fields', {})

        # Extract basic issue information
        # v3 API returns description as ADF (Atlassian Document Format), convert to plain text
        description = fields.get('description')
        if isinstance(description, dict):
            description = self._adf_to_text(description)

        issue_info = {
            "key": issue_data.get('key'),
            "summary": fields.get('summary'),
            "description": description,
            "status": fields.get('status', {}).get('name'),
            "priority": fields.get('priority', {}).get('name'),
            "issue_type": fields.get('issuetype', {}).get('name'),
            "assignee": fields.get('assignee', {}).get('displayName') if fields.get('assignee') else None,
            "reporter": fields.get('reporter', {}).get('displayName') if fields.get('reporter') else None,
            "created": fields.get('created'),
            "updated": fields.get('updated'),
            "resolution": fields.get('resolution', {}).get('name') if fields.get('resolution') else None,
            "fix_versions": [v.get('name') for v in fields.get('fixVersions', [])],
            "labels": fields.get('labels', []),
            "components": [c.get('name') for c in fields.get('components', [])],
        }

        # Process comments
        comments = comments_data.get('comments', [])
        processed_comments = []

        for comment in comments:
            body = comment.get('body')
            if isinstance(body, dict):
                body = self._adf_to_text(body)

            comment_info = {
                "author": comment.get('author', {}).get('displayName'),
                "body": body,
                "created": comment.get('created'),
                "updated": comment.get('updated'),
            }
            processed_comments.append(comment_info)

        # Sort comments by creation date (newest first)
        processed_comments.sort(key=lambda x: x['created'], reverse=True)

        # Calculate days since last comment
        days_since_last_comment = None
        if processed_comments:
            days_since_last_comment = self._get_days_old(processed_comments[0]['created'])

        return {
            "issue": issue_info,
            "comments": {
                "total_count": len(processed_comments),
                "recent_comments": processed_comments[:10],  # Last 10 comments
                "days_since_last_comment": days_since_last_comment,
            },
        }

    def _adf_to_text(self, adf: Dict[str, Any]) -> str:
        """Convert Atlassian Document Format to plain text."""
        if not isinstance(adf, dict):
            return str(adf) if adf else ""

        parts = []
        for node in adf.get('content', []):
            node_type = node.get('type', '')
            if node_type == 'text':
                parts.append(node.get('text', ''))
            elif 'content' in node:
                parts.append(self._adf_to_text(node))
            elif node_type == 'hardBreak':
                parts.append('\n')
        return ''.join(parts)

    def _get_days_old(self, date_string: str) -> int:
        """Calculate how many days old a date string is."""
        try:
            # Parse the date string (Jira uses ISO format)
            date_obj = datetime.fromisoformat(date_string.replace('Z', '+00:00'))
            now = datetime.now(timezone.utc)
            delta = now - date_obj
            return delta.days
        except Exception:
            return 999  # Return a large number for unparseable dates
