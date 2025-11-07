"""
Tool for getting OpenShift release payload information from the release controller API.
"""

import json
import logging
import re
from typing import Any, Dict, List, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyReleasePayloadTool(SippyBaseTool):
    """Tool for getting OpenShift release payload information."""

    name: str = "get_release_payloads"
    description: str = "Get a JSON object containing a list of recent OpenShift release payloads with their status. Use this to find the name of the latest payload. For specific payload details, use get_payload_details. Input: release version (e.g., '4.20') and optional stream type ('nightly' or 'ci', defaults to 'nightly')"

    # Release controller API base URL
    release_controller_url: str = Field(
        default="https://amd64.ocp.releases.ci.openshift.org/api/v1", description="Release controller API base URL"
    )
    # TODO: this should probably be switched to use the sippy API for payloads, which is permanent whereas release controller will prune

    class ReleasePayloadInput(SippyToolInput):
        release_version: str = Field(description="Release version (e.g., '4.20', '4.19')")
        stream_type: Optional[str] = Field(default="nightly", description="Stream type: 'nightly' or 'ci' (defaults to 'nightly')")

    args_schema: Type[SippyToolInput] = ReleasePayloadInput

    def _run(
        self,
        *args,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Get release payload information from the release controller API."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        # Pydantic model will have validated and filled in defaults
        args = self.ReleasePayloadInput(**input_data)

        # Validate and clean inputs
        stream_type = args.stream_type or "nightly"
        if stream_type not in ["nightly", "ci"]:
            return {"error": f"Invalid stream type '{stream_type}'. Must be 'nightly' or 'ci'."}

        # Clean release version (remove any extra characters)
        clean_version = re.sub(r"[^\d\.]", "", args.release_version)
        if not re.match(r"^\d+\.\d+$", clean_version):
            return {"error": f"Invalid release version format. Expected format like '4.20', got: {args.release_version}"}

        # Construct the release stream name
        release_stream = f"{clean_version}.0-0.{stream_type}"

        # Construct the API endpoint
        endpoint = f"{self.release_controller_url.rstrip('/')}/releasestream/{release_stream}/tags"

        try:
            logger.info(f"Making request to {endpoint}")

            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint)
                response.raise_for_status()

                data = response.json()

                # Return the raw JSON data
                return data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting release payloads: {e}")
            if e.response.status_code == 404:
                return {"error": f"Release stream '{release_stream}' not found. Check if the release version and stream type are correct."}
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error getting release payloads: {e}")
            return {"error": f"Failed to connect to release controller API - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from release controller API"}
        except Exception as e:
            logger.error(f"Unexpected error getting release payloads: {e}")
            return {"error": f"Unexpected error - {str(e)}"}

    def get_latest_payload(self, release_version: str, stream_type: str = "nightly") -> Optional[Dict[str, Any]]:
        """Helper method to get just the latest payload information."""
        try:
            # Use the main _run method but parse the result differently
            # This is a simplified version for programmatic access
            clean_version = re.sub(r"[^\d\.]", "", release_version)
            release_stream = f"{clean_version}.0-0.{stream_type}"
            endpoint = f"{self.release_controller_url.rstrip('/')}/releasestream/{release_stream}/tags"

            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint)
                response.raise_for_status()
                data = response.json()

                tags = data.get("tags", [])
                if not tags:
                    return None

                # Find first non-Ready payload
                for tag in tags:
                    if tag.get("phase", "").lower() != "ready":
                        return tag

                # If all are Ready, return the first one
                return tags[0] if tags else None

        except Exception as e:
            logger.error(f"Error getting latest payload: {e}")
            return None
