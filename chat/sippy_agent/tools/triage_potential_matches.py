"""
Tool for getting potential matches for a triage.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class TriagePotentialMatchesTool(SippyBaseTool):
    name: str = "get_triage_potential_matches"
    description: str = """Get potential matching regressions for a triage record.
    
This tool returns a list of tests that might belong to the same triage based on:
- Similar test names (edit distance scoring)
- Same last failure times (tests that fail in the same job runs)
It includes a Confidence level (1-10, higher is better)

Each potential match includes a test_details_api_url that you can use with the 
get_test_details_report tool to analyze the test's failure patterns and compare
them with existing triaged tests.

Input: triage_id (the triage ID) and view (the component readiness view, e.g., '4.20-main')"""

    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")

    class PotentialMatchesInput(SippyToolInput):
        triage_id: int = Field(description="The triage ID to find potential matches for")
        view: str = Field(description="The component readiness view (e.g., '4.20-main')")

    args_schema: Type[SippyToolInput] = PotentialMatchesInput

    def _run(self, *args, **kwargs: Any) -> Dict[str, Any]:
        """Get potential matches from the Sippy API."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        args = self.PotentialMatchesInput(**input_data)

        api_url = self.sippy_api_url
        if not api_url:
            return {
                "error": "No Sippy API URL configured. Please set SIPPY_API_URL environment variable."
            }

        try:
            # Get potential matches
            potential_matches_url = f"{api_url.rstrip('/')}/api/component_readiness/triages/{args.triage_id}/matches?view={args.view}"
            logger.info(f"Fetching potential matches from {potential_matches_url}")
            
            with httpx.Client(timeout=30.0) as client:
                matches_response = client.get(potential_matches_url)
                matches_response.raise_for_status()
                potential_matches = matches_response.json()

            if not potential_matches:
                return {
                    "message": f"No potential matches found for triage {args.triage_id} in view '{args.view}'",
                    "potential_matches": []
                }

            # Sort by confidence level (descending)
            potential_matches.sort(key=lambda x: x.get('confidence_level', 0), reverse=True)

            return {
                "triage_id": args.triage_id,
                "view": args.view,
                "potential_matches_count": len(potential_matches),
                "potential_matches": potential_matches,
            }

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting potential matches: {e}")
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error getting potential matches: {e}")
            return {"error": f"Failed to connect to Sippy API at {api_url} - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from Sippy API"}
        except Exception as e:
            logger.error(f"Unexpected error getting potential matches: {e}", exc_info=True)
            return {"error": f"Unexpected error - {str(e)}"}
