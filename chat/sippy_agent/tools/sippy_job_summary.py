"""
Tool for getting prow job run summaries from Sippy API.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput
from .test_analysis_helpers import analyze_test_failures, extract_test_category, clean_failure_message

logger = logging.getLogger(__name__)


class SippyProwJobSummaryTool(SippyBaseTool):
    """Tool for getting prow job run summaries from Sippy API."""
    
    name: str = "get_prow_job_summary"
    description: str = "Get a summary of a prow job run including URL, TestGrid URL, timing, results, and test failures. Contains all basic job information. Input: just the numeric job ID (e.g., 1934795512955801600)"
    
    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")
    
    class ProwJobSummaryInput(SippyToolInput):
        prow_job_run_id: str = Field(description="Numeric prow job run ID only (e.g., 1934795512955801600)")
        sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL (optional, uses config if not provided)")
    
    args_schema: Type[SippyToolInput] = ProwJobSummaryInput
    
    def _run(self, prow_job_run_id: str, sippy_api_url: Optional[str] = None) -> str:
        """Get prow job run summary from Sippy API."""
        # Use provided URL or fall back to instance URL
        api_url = sippy_api_url or self.sippy_api_url
        
        if not api_url:
            return "Error: No Sippy API URL configured. Please set SIPPY_API_URL environment variable or provide sippy_api_url parameter."
        
        # Clean and validate the job ID - extract just the numeric part
        clean_job_id = str(prow_job_run_id).strip()
        # Extract just the numeric part if there's extra text
        import re
        job_id_match = re.search(r'\b(\d{10,})\b', clean_job_id)
        if job_id_match:
            clean_job_id = job_id_match.group(1)
        elif not clean_job_id.isdigit():
            return f"Error: Invalid job ID format. Expected numeric ID, got: {prow_job_run_id}"
        
        # Construct the API endpoint
        endpoint = f"{api_url.rstrip('/')}/api/job/run/summary"
        
        try:
            # Make the API request
            params = {"prow_job_run_id": clean_job_id}
            logger.info(f"Making request to {endpoint} with params: {params}")
            
            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint, params=params)
                response.raise_for_status()
                
                data = response.json()
                
                # Format the response for better readability
                return self._format_job_summary(data)
                
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting job summary: {e}")
            return f"Error: HTTP {e.response.status_code} - {e.response.text}"
        except httpx.RequestError as e:
            logger.error(f"Request error getting job summary: {e}")
            return f"Error: Failed to connect to Sippy API at {api_url} - {str(e)}"
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return f"Error: Invalid JSON response from Sippy API"
        except Exception as e:
            logger.error(f"Unexpected error getting job summary: {e}")
            return f"Error: Unexpected error - {str(e)}"

    def _format_job_summary(self, data: Dict[str, Any]) -> str:
        """Format the job summary data for display."""
        if not data:
            return "No data returned from Sippy API"

        # Extract main fields
        job_id = data.get("id", "Unknown")
        job_name = data.get("name", "Unknown")
        release = data.get("release", "Unknown")
        cluster = data.get("cluster", "Unknown")
        start_time = data.get("startTime", "")
        duration_seconds = data.get("durationSeconds", 0)
        overall_result = data.get("overallResult", "Unknown")
        reason = data.get("reason", "Unknown")
        succeeded = data.get("succeeded", False)
        failed = data.get("failed", False)
        infrastructure_failure = data.get("infrastructureFailure", False)
        known_failure = data.get("knownFailure", False)
        test_count = data.get("testCount", 0)
        test_failure_count = data.get("testFailureCount", 0)
        variants = data.get("variants", [])
        url = data.get("url", "")
        testgrid_url = data.get("testGridURL", "")

        # Legacy fields for backward compatibility
        test_failures = data.get("testFailures", {})
        degraded_operators = data.get("degradedOperators", {})

        # Build formatted response
        result = f"**Prow Job Summary**\n\n"
        result += f"**Job ID:** {job_id}\n"
        result += f"**Job Name:** {job_name}\n"
        result += f"**Release:** {release}\n"
        result += f"**Cluster:** {cluster}\n\n"

        # Check if this is an aggregated job and provide basic information
        if job_name and job_name.startswith("aggregated-"):
            result += f"🔄 **AGGREGATED JOB DETECTED**\n"
            result += f"This is a statistical aggregation job that runs multiple instances (typically 10) of the same test.\n"
            result += f"The test failures shown below are from the aggregated results.\n\n"

        # Format timing information
        result += f"**⏱️ Timing & Duration:**\n"
        if start_time:
            # Parse and format the start time
            formatted_start = self._format_timestamp(start_time)
            result += f"Start Time: {formatted_start}\n"

        if duration_seconds > 0:
            formatted_duration = self._format_duration(duration_seconds)
            result += f"Duration: {formatted_duration} ({duration_seconds:,} seconds)\n"
        else:
            result += f"Duration: Not available\n"
        result += "\n"

        # Format results
        result += f"**📊 Results:**\n"
        result += f"Overall Result: {overall_result}\n"
        result += f"Succeeded: {'✅ Yes' if succeeded else '❌ No'}\n"
        result += f"Failed: {'❌ Yes' if failed else '✅ No'}\n"
        result += f"Infrastructure Failure: {'🚨 Yes' if infrastructure_failure else '✅ No'}\n"
        result += f"Known Failure: {'⚠️ Yes' if known_failure else '✅ No'}\n"
        result += f"Reason: {reason}\n\n"

        # Format test information
        result += f"**🧪 Test Information:**\n"
        result += f"Total Tests: {test_count}\n"
        result += f"Failed Tests: {test_failure_count}\n"
        if test_failure_count > 0 and test_count > 0:
            failure_rate = (test_failure_count / test_count) * 100
            result += f"Failure Rate: {failure_rate:.1f}%\n"
        result += "\n"

        # Format variants
        if variants:
            result += f"**🔧 Configuration Variants:**\n"
            # Group variants by type
            variant_groups = {}
            for variant in variants:
                if ':' in variant:
                    key, value = variant.split(':', 1)
                    variant_groups[key] = value
                else:
                    variant_groups['Other'] = variant_groups.get('Other', []) + [variant]

            for key, value in variant_groups.items():
                if isinstance(value, list):
                    result += f"{key}: {', '.join(value)}\n"
                else:
                    result += f"{key}: {value}\n"
            result += "\n"

        # Format legacy test failures if present (limit to 25 to control token usage)
        if test_failures:
            total_failures = len(test_failures)
            max_failures_to_show = 25

            result += f"**❌ Failed Tests Details ({total_failures} total"
            if total_failures > max_failures_to_show:
                result += f", showing first {max_failures_to_show}"
            result += "):**\n"

            # Analyze test failure patterns (use all failures for analysis)
            test_analysis = analyze_test_failures(test_failures)
            if test_analysis:
                result += f"\n**🔍 Test Failure Analysis:**\n{test_analysis}\n"

            result += f"\n**📋 Individual Test Failures:**\n"

            # Limit the number of individual failures displayed
            failures_to_show = list(test_failures.items())[:max_failures_to_show]

            for i, (test_name, failure_msg) in enumerate(failures_to_show, 1):
                # Extract test category from test name
                test_category = extract_test_category(test_name)

                # Truncate very long failure messages but keep key error info
                clean_msg = clean_failure_message(failure_msg)

                result += f"{i}. **{test_name}**\n"
                if test_category:
                    result += f"   Category: {test_category}\n"
                result += f"   Error: {clean_msg}\n\n"

            # Add note if there are more failures
            if total_failures > max_failures_to_show:
                remaining = total_failures - max_failures_to_show
                result += f"... and {remaining} more failed tests (use log analysis tools for detailed investigation)\n\n"

        # Format degraded operators if present (limit to 10 to control token usage)
        if degraded_operators:
            total_operators = len(degraded_operators)
            max_operators_to_show = 10

            result += f"**⚠️ Degraded Operators ({total_operators} total"
            if total_operators > max_operators_to_show:
                result += f", showing first {max_operators_to_show}"
            result += "):**\n"

            # Limit the number of operators displayed
            operators_to_show = list(degraded_operators.items())[:max_operators_to_show]

            for i, (operator_name, operator_info) in enumerate(operators_to_show, 1):
                result += f"{i}. **{operator_name}**\n"
                if isinstance(operator_info, str):
                    result += f"   Info: {operator_info}\n"
                else:
                    result += f"   Info: {str(operator_info)}\n"
                result += "\n"

            # Add note if there are more operators
            if total_operators > max_operators_to_show:
                remaining = total_operators - max_operators_to_show
                result += f"... and {remaining} more degraded operators\n\n"

        # Add useful links
        result += f"**🔗 Links:**\n"
        if url:
            result += f"**Prow Job URL:** {url}\n"
            result += f"[View Job in Prow]({url})\n"
        if testgrid_url:
            result += f"**TestGrid URL:** {testgrid_url}\n"
            result += f"[View in TestGrid]({testgrid_url})\n"

        return result

    def _format_timestamp(self, timestamp: str) -> str:
        """Format timestamp to a more readable format."""
        try:
            from datetime import datetime
            # Handle timezone offset format like "2025-06-16T22:09:31-04:00"
            if timestamp.endswith('Z'):
                dt = datetime.fromisoformat(timestamp[:-1])
            elif '+' in timestamp[-6:] or '-' in timestamp[-6:]:
                # Remove timezone for simple parsing
                dt = datetime.fromisoformat(timestamp[:-6])
            else:
                dt = datetime.fromisoformat(timestamp)
            return dt.strftime('%Y-%m-%d %H:%M:%S UTC')
        except Exception:
            return timestamp

    def _format_duration(self, seconds: int) -> str:
        """Format duration in seconds to a human-readable format."""
        if seconds < 60:
            return f"{seconds}s"
        elif seconds < 3600:
            minutes = seconds // 60
            remaining_seconds = seconds % 60
            return f"{minutes}m {remaining_seconds}s"
        else:
            hours = seconds // 3600
            remaining_minutes = (seconds % 3600) // 60
            remaining_seconds = seconds % 60
            if remaining_seconds > 0:
                return f"{hours}h {remaining_minutes}m {remaining_seconds}s"
            else:
                return f"{hours}h {remaining_minutes}m"
