"""
Tool for analyzing Jira issues and their comments.
"""

import json
import logging
from typing import Any, Dict, Type
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

    class JiraIssueInput(SippyToolInput):
        issue_key: str = Field(description="Jira issue key (e.g., OCPBUGS-12345)")

    args_schema: Type[SippyToolInput] = JiraIssueInput

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
            # Make API request to get issue details using configured base URL
            api_url = f"{self.jira_url.rstrip('/')}/rest/api/2/issue/{issue_key}"
            
            logger.info(f"Making request to {api_url}")
            
            with httpx.Client(timeout=30.0) as client:
                # Get issue details (no authentication needed for public data)
                response = client.get(api_url)
                response.raise_for_status()
                
                issue_data = response.json()
                
                # Get comments (no authentication needed for public data)
                comments_response = client.get(f"{api_url}/comment")
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
        issue_info = {
            "key": issue_data.get('key'),
            "summary": fields.get('summary'),
            "description": fields.get('description'),
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
            comment_info = {
                "author": comment.get('author', {}).get('displayName'),
                "body": comment.get('body'),
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
