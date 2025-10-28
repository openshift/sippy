"""
Tool for getting test details reports from Sippy API.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)

# TODO(sgoeddel): eventually, TestDetailsReport.js should utilize this tool directly and contain minimal other instructions
# It is very difficult to obtain similar functionality that way, however. For now, this is used on other pages only.
class SippyTestDetailsTool(SippyBaseTool):
    """Tool for getting comprehensive test details reports from Sippy API."""

    name: str = "get_test_details_report"
    description: str = """Get a test details report from Sippy API including regression analysis and statistics.
    
This tool provides:
- Regression status and history
- Sample vs base statistics comparison
- Job stats for each job name that matched the variants in the report
  - List of sample job runs and basis job runs, sorted by start time with most recent first
  - Whether each job run was a success or failure (see failure_count or success_count > 0)
  - Simplified job run data with job_url, job_run_id, start_time, and status (passed/failed/flaked)
  - get_prow_job_summary can be used to dig deeper into specific job runs by job run ID
- Triage information
- Pass rate changes

Input: query_params (the query parameters for the test details endpoint, e.g., testId=12345&component=...&baseRelease=...)"""

    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")

    class TestDetailsInput(SippyToolInput):
        query_params: str = Field(description="Query parameters for the test details endpoint. Can be either just the query params (e.g., testId=12345&component=foo) or a full URL (the query params will be extracted)")

    args_schema: Type[SippyToolInput] = TestDetailsInput

    def _run(self, *args, **kwargs: Any) -> Dict[str, Any]:
        """Get test details report from Sippy API."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        # Pydantic model will have validated and filled in defaults
        args = self.TestDetailsInput(**input_data)

        # Use the configured API URL
        api_url = self.sippy_api_url

        if not api_url:
            return {
                "error": "No Sippy API URL configured. Please set SIPPY_API_URL environment variable."
            }

        # Build the full URL from query params
        query_params = args.query_params.strip()
        
        # If query_params is a full URL, extract just the query string
        if query_params.startswith('http') or query_params.startswith('/'):
            # Extract query params from URL
            if '?' in query_params:
                query_params = query_params.split('?', 1)[1]
            else:
                return {"error": f"No query parameters found in URL: {query_params}"}
        
        # Ensure query params don't start with ? or &
        if query_params.startswith('?') or query_params.startswith('&'):
            query_params = query_params[1:]
        
        full_url = f"{api_url.rstrip('/')}/api/component_readiness/test_details?{query_params}"

        try:
            # Make the API request
            logger.info(f"Making request to {full_url}")

            with httpx.Client(timeout=60.0) as client:
                response = client.get(full_url)
                response.raise_for_status()

                data = response.json()

                # Check if the response indicates an error
                if data.get('code') and (data['code'] < 200 or data['code'] >= 300):
                    return {
                        "error": f"API returned error code {data['code']}: {data.get('message', 'Unknown error')}"
                    }

                # Process the response to extract key information
                processed_data = self._process_test_details_response(data, api_url)
                
                # Return the processed data
                return processed_data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting test details: {e}")
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error getting test details: {e}")
            return {"error": f"Failed to connect to Sippy API at {api_url} - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from Sippy API"}
        except Exception as e:
            logger.error(f"Unexpected error getting test details: {e}")
            return {"error": f"Unexpected error - {str(e)}"}

    def _process_test_details_response(self, data: Dict[str, Any], api_url: str) -> Dict[str, Any]:
        """Process the raw API response to extract and structure key information."""
        
        if not data.get('analyses') or not data['analyses']:
            return {
                "error": "No analysis data found in response",
                "raw_data": data
            }

        first_analysis = data['analyses'][0]

        # Extract failed job run IDs
        failed_job_run_ids = []
        if first_analysis.get('job_stats'):
            for job_stat in first_analysis['job_stats']:
                if job_stat.get('sample_job_run_stats'):
                    for sample_job_run in job_stat['sample_job_run_stats']:
                        if (sample_job_run.get('test_stats') and 
                            sample_job_run['test_stats'].get('failure_count', 0) > 0 and
                            sample_job_run.get('job_run_id')):
                            failed_job_run_ids.append(sample_job_run['job_run_id'])
                            if len(failed_job_run_ids) >= 10:  # Limit to 10
                                break
                    if len(failed_job_run_ids) >= 10:
                        break

        # Process job stats to create simplified job run data
        job_stats = {}
        if first_analysis.get('job_stats'):
            for job_stat in first_analysis['job_stats']:
                sample_job_name = job_stat.get('sample_job_name')
                if sample_job_name and job_stat.get('sample_job_run_stats'):
                    job_runs = []
                    for job_run in job_stat['sample_job_run_stats']:
                        # Determine status based on test stats counts
                        test_stats = job_run.get('test_stats', {})
                        status = self._determine_job_run_status(test_stats)
                        
                        job_run_data = {
                            "job_url": job_run.get('job_url', ''),
                            "job_run_id": job_run.get('job_run_id', ''),
                            "start_time": job_run.get('start_time', ''),
                            "status": status
                        }
                        job_runs.append(job_run_data)
                    
                    job_stats[sample_job_name] = job_runs

        # Process regression information
        regression_info = None
        if first_analysis.get('regression'):
            regression = first_analysis['regression']
            regression_info = {
                "id": regression.get('id'),
                "opened": regression.get('opened'),
                "closed": regression.get('closed', {}).get('time') if regression.get('closed', {}).get('valid') else None,
            }

        return {
            "test_name": data.get('test_name'),
            "test_id": data.get('test_id'),
            "component": data.get('component'),
            "capability": data.get('capability'),
            "environment": data.get('environment'),
            "regression": regression_info,
            "status": first_analysis.get('status'),
            "explanations": first_analysis.get('explanations', []),
            "sample_stats": first_analysis.get('sample_stats'),
            "base_stats": first_analysis.get('base_stats'),
            "failed_job_run_ids": failed_job_run_ids,
            "job_stats": job_stats,
            "triages_count": len(first_analysis.get('triages', [])),
            "generated_at": data.get('generated_at'),
        }

    def _determine_job_run_status(self, test_stats: Dict[str, Any]) -> str:
        """Determine the status of a job run based on test stats counts.
        
        Args:
            test_stats: Dictionary containing success_count, failure_count, and flake_count
            
        Returns:
            String indicating status: 'passed', 'failed', or 'flaked'
        """
        success_count = test_stats.get('success_count', 0)
        failure_count = test_stats.get('failure_count', 0)
        flake_count = test_stats.get('flake_count', 0)
        
        # Determine status based on which count is > 0
        # Priority: failure > flake > success
        if failure_count > 0:
            return 'failed'
        elif flake_count > 0:
            return 'flaked'
        elif success_count > 0:
            return 'passed'
        else:
            # If all counts are 0, default to 'passed' (no test results)
            return 'passed'
