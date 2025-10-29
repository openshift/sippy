"""
Tool for getting detailed OpenShift release payload information from the release controller API.
"""

import json
import logging
import re
from typing import Any, Dict, List, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyPayloadDetailsTool(SippyBaseTool):
    """Tool for getting detailed OpenShift release payload information."""

    name: str = "get_payload_details"
    description: str = "Get a JSON object with comprehensive information for a specific OpenShift release payload. Use this ONLY when user asks for details about a specific payload. For basic payload status, use get_release_payloads first. Input: payload name (e.g., '4.20.0-0.nightly-2025-06-17-061341')"

    # Release controller API base URL
    release_controller_url: str = Field(
        default="https://amd64.ocp.releases.ci.openshift.org/api/v1", description="Release controller API base URL"
    )
    # TODO: this should probably be switched to use the sippy API for payloads, which is permanent whereas release controller will prune

    # Sippy API URL for job analysis
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL for job analysis")

    class PayloadDetailsInput(SippyToolInput):
        payload_name: str = Field(description="Full payload name (e.g., '4.20.0-0.nightly-2025-06-17-061341')")
        include_job_analysis: Optional[bool] = Field(
            default=False, description="Include suggested next steps for analyzing failed blocking jobs"
        )
        max_jobs_to_analyze: Optional[int] = Field(
            default=5, description="Maximum number of failed jobs to analyze in detail (defaults to 5 to avoid excessive API calls)"
        )

    args_schema: Type[SippyToolInput] = PayloadDetailsInput

    def _run(
        self,
        *args,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Get detailed payload information from the release controller API."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        # Pydantic model will have validated and filled in defaults
        args = self.PayloadDetailsInput(**input_data)

        # Clean the payload name in case it includes parameter syntax
        clean_payload_name = self._clean_payload_name(args.payload_name)

        # Extract release stream from payload name
        release_stream = self._extract_release_stream(clean_payload_name)
        if not release_stream:
            return {
                "error": f"Error: Could not extract release stream from payload name '{clean_payload_name}'. Expected format like '4.20.0-0.nightly-2025-06-17-061341'"
            }

        # Construct the API endpoint for payload details
        endpoint = f"{self.release_controller_url.rstrip('/')}/releasestream/{release_stream}/release/{clean_payload_name}"

        try:
            logger.info(f"Making request to {endpoint}")

            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint)
                response.raise_for_status()

                # Log response details for debugging
                logger.debug(f"Response status: {response.status_code}")
                logger.debug(f"Response content type: {response.headers.get('content-type', 'unknown')}")

                # Check if response is JSON (be more lenient with content type checking)
                content_type = response.headers.get("content-type", "")
                if not (
                    content_type.startswith("application/json")
                    or content_type.startswith("text/json")
                    or response.text.strip().startswith("{")
                ):
                    logger.warning(f"Unexpected content type: {content_type}")
                    logger.warning(f"Response text: {response.text[:500]}...")
                    return {"error": f"API returned non-JSON response. Content-Type: {content_type}"}

                try:
                    data = response.json()
                    logger.debug(f"JSON parsed successfully, type: {type(data)}")
                except json.JSONDecodeError as json_err:
                    logger.error(f"JSON decode error: {json_err}")
                    logger.error(f"Response text: {response.text[:500]}...")
                    return {"error": f"Invalid JSON response from API. Response: {response.text[:200]}..."}

                # Validate that data is a dictionary
                if not isinstance(data, dict):
                    logger.error(f"Expected dict, got {type(data)}: {str(data)[:200]}...")
                    return {"error": f"API returned unexpected data type {type(data)}. Expected JSON object."}

                # Remove the redundant base64 changelog
                if "changeLog" in data:
                    del data["changeLog"]

                # Return the raw JSON data
                return data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting payload details: {e}")
            if e.response.status_code == 404:
                return {
                    "error": f"Payload '{clean_payload_name}' not found in release stream '{release_stream}'. Check if the payload name is correct."
                }
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error getting payload details: {e}")
            return {"error": f"Failed to connect to release controller API - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from release controller API"}
        except Exception as e:
            logger.error(f"Unexpected error getting payload details: {e}")
            return {"error": f"Unexpected error - {str(e)}"}

    def _clean_payload_name(self, payload_name: str) -> str:
        """Clean payload name from common parameter syntax issues."""
        # Remove common parameter syntax patterns
        cleaned = payload_name.strip()

        # Handle cases like "payload name = '4.20.0-0.nightly-2025-06-17-061341'"
        if "=" in cleaned:
            cleaned = cleaned.split("=")[-1].strip()

        # Remove quotes
        cleaned = cleaned.strip("'\"")

        # Extract just the payload name pattern
        payload_pattern = re.search(r"(\d+\.\d+\.0-0\.(nightly|ci)-\d{4}-\d{2}-\d{2}-\d{6})", cleaned)
        if payload_pattern:
            return payload_pattern.group(1)

        return cleaned

    def _extract_release_stream(self, payload_name: str) -> Optional[str]:
        """Extract release stream from payload name."""
        # Expected format: 4.20.0-0.nightly-2025-06-17-061341
        match = re.match(r"^(\d+\.\d+\.0-0\.(nightly|ci))-\d{4}-\d{2}-\d{2}-\d{6}$", payload_name)
        if match:
            return match.group(1)
        return None
