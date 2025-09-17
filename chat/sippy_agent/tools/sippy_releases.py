"""
Tool for getting OpenShift release information from Sippy API.
"""

import json
import logging
from datetime import datetime
from typing import Any, Dict, List, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyReleasesTool(SippyBaseTool):
    """Tool for getting OpenShift release information from Sippy API."""

    name: str = "get_release_info"
    description: str = "Get OpenShift release information including available releases, GA dates, and development start dates. Returns all release data from the Sippy API. No parameters required."

    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")

    class ReleasesInput(SippyToolInput):
        pass  # No parameters needed

    args_schema: Type[SippyToolInput] = ReleasesInput

    def _run(self, *args, **kwargs: Any) -> str:
        """Get release information from Sippy API."""
        if not self.sippy_api_url:
            return "Error: No Sippy API URL configured. Please set SIPPY_API_URL environment variable."
        
        # Construct the API endpoint
        endpoint = f"{self.sippy_api_url.rstrip('/')}/api/releases"
        
        try:
            logger.info(f"Making request to {endpoint}")
            
            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint)
                response.raise_for_status()
                
                data = response.json()

                # Always return all releases data
                return self._format_all_releases_response(data)
                
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting release info: {e}")
            return f"Error: HTTP {e.response.status_code} - {e.response.text}"
        except httpx.RequestError as e:
            logger.error(f"Request error getting release info: {e}")
            return f"Error: Failed to connect to Sippy API at {self.sippy_api_url} - {str(e)}"
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return f"Error: Invalid JSON response from Sippy API"
        except Exception as e:
            logger.error(f"Unexpected error getting release info: {e}")
            return f"Error: Unexpected error - {str(e)}"
    
    def _format_all_releases_response(self, data: Dict[str, Any]) -> str:
        """Format the release response data showing all releases."""
        if not data:
            return "No data returned from Sippy API"

        releases = data.get("releases", [])
        ga_dates = data.get("ga_dates", {})
        dates = data.get("dates", {})
        last_updated = data.get("last_updated", "")

        # Filter out non-release entries like "Presubmits"
        filtered_releases = [r for r in releases if r != "Presubmits"]

        if not filtered_releases:
            return "No valid releases found in Sippy API response"

        # Always return comprehensive release information
        return self._format_all_releases(filtered_releases, ga_dates, dates, last_updated)

    def _format_all_releases(
        self,
        releases: List[str],
        ga_dates: Dict[str, str],
        dates: Dict[str, Dict[str, str]],
        last_updated: str
    ) -> str:
        """Format response showing all releases."""
        result = f"**📋 All OpenShift Releases**\n\n"
        result += f"**Total Releases:** {len(releases)}\n"
        
        if last_updated:
            formatted_updated = self._format_date(last_updated)
            result += f"**Last Updated:** {formatted_updated}\n"
        
        result += f"\n**Release List:**\n"
        
        for i, release in enumerate(releases, 1):
            ga_date = ga_dates.get(release)
            status_emoji = "✅" if ga_date else "🚧"
            status_text = "GA" if ga_date else "Dev"
            
            result += f"{i:2d}. **{release}** {status_emoji} {status_text}"
            
            if ga_date:
                formatted_ga = self._format_date(ga_date)
                result += f" (GA: {formatted_ga})"
            else:
                # Show dev start if available
                release_dates = dates.get(release, {})
                dev_start = release_dates.get("development_start")
                if dev_start:
                    formatted_dev = self._format_date(dev_start)
                    result += f" (Dev: {formatted_dev})"
            
            result += "\n"
        
        return result

    def _format_date(self, date_str: str) -> str:
        """Format ISO date string to readable format."""
        try:
            # Parse ISO format date
            dt = datetime.fromisoformat(date_str.replace('Z', '+00:00'))
            return dt.strftime('%Y-%m-%d')
        except Exception:
            # Return original if parsing fails
            return date_str
