"""
Tool for parsing aggregated test results in YAML format.
"""

import logging
from typing import Any, Dict, Optional, Type, List
from pydantic import Field
import httpx
import yaml

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class AggregatedYAMLParserTool(SippyBaseTool):
    """Tool for parsing aggregated test results from YAML format."""
    
    name: str = "parse_aggregated_yaml"
    description: str = "Parse aggregated test results from YAML format. Takes yaml_url as required parameter. Only use for aggregated jobs."
    
    class AggregatedYAMLInput(SippyToolInput):
        yaml_url: str = Field(description="URL to the aggregated YAML file")
    
    args_schema: Type[SippyToolInput] = AggregatedYAMLInput
    
    def _run(self, yaml_url: str) -> str:
        """Parse aggregated YAML file and extract test results with underlying job links."""
        try:
            # Fetch the YAML content
            logger.info(f"Fetching aggregated YAML from: {yaml_url}")
            
            with httpx.Client(timeout=60.0) as client:
                response = client.get(yaml_url)
                response.raise_for_status()
                
                yaml_content = response.text
                
            # Parse the YAML
            try:
                data = yaml.safe_load(yaml_content)
            except yaml.YAMLError as e:
                logger.error(f"YAML parse error: {e}")
                return f"Error: Invalid YAML format - {str(e)}"
            
            # Extract and format the results
            return self._format_aggregated_results(data)
                
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error fetching aggregated YAML: {e}")
            return f"Error: HTTP {e.response.status_code} - Failed to fetch YAML from {yaml_url}"
        except httpx.RequestError as e:
            logger.error(f"Request error fetching aggregated YAML: {e}")
            return f"Error: Failed to connect to {yaml_url} - {str(e)}"
        except Exception as e:
            logger.error(f"Unexpected error parsing aggregated YAML: {e}")
            return f"Error: Unexpected error - {str(e)}"
    
    def _format_aggregated_results(self, data: Any) -> str:
        """Format aggregated test results for display."""
        if not isinstance(data, dict):
            return "Error: Expected YAML data to be a dictionary"
        
        result = "**🔄 Aggregated Test Results**\n\n"
        
        # Extract basic information
        testsuitename = data.get('testsuitename', 'Unknown')
        summary = data.get('summary', 'No summary available')
        
        result += f"**Test Suite:** {testsuitename}\n"
        result += f"**Summary:** {summary}\n\n"
        
        # Process passes
        passes = data.get('passes', [])
        failures = data.get('failures', [])
        skips = data.get('skips', [])
        
        if passes:
            result += f"**✅ Passing Jobs ({len(passes)} total):**\n"
            for i, job in enumerate(passes[:5], 1):  # Show first 5
                job_id = job.get('jobrunid', 'Unknown')
                human_url = job.get('humanurl', 'No URL')
                result += f"{i}. Job ID {job_id}: {human_url}\n"
            
            if len(passes) > 5:
                result += f"... and {len(passes) - 5} more passing jobs\n"
            result += "\n"
        
        if failures:
            result += f"**❌ Failing Jobs ({len(failures)} total):**\n"
            for i, job in enumerate(failures, 1):
                job_id = job.get('jobrunid', 'Unknown')
                human_url = job.get('humanurl', 'No URL')
                gcs_url = job.get('gcsartifacturl', 'No artifacts URL')
                result += f"{i}. **Job ID {job_id}**\n"
                result += f"   🔗 Job URL: {human_url}\n"
                result += f"   📁 Artifacts: {gcs_url}\n\n"
            
            result += "💡 **For deep analysis:** Use the job summary tool on individual failing job IDs above to analyze specific failures.\n\n"
        
        if skips:
            result += f"**⏭️ Skipped Jobs ({len(skips)} total):**\n"
            for i, job in enumerate(skips[:3], 1):  # Show first 3
                job_id = job.get('jobrunid', 'Unknown')
                human_url = job.get('humanurl', 'No URL')
                result += f"{i}. Job ID {job_id}: {human_url}\n"
            
            if len(skips) > 3:
                result += f"... and {len(skips) - 3} more skipped jobs\n"
            result += "\n"
        
        # Add analysis summary
        total_jobs = len(passes) + len(failures) + len(skips)
        if total_jobs > 0:
            pass_rate = (len(passes) / total_jobs) * 100
            result += f"**📊 Analysis Summary:**\n"
            result += f"- Total jobs: {total_jobs}\n"
            result += f"- Pass rate: {pass_rate:.1f}% ({len(passes)}/{total_jobs})\n"
            result += f"- Failures: {len(failures)}\n"
            result += f"- Skips: {len(skips)}\n\n"
        
        # Extract historical context from summary if available
        if 'historical pass rate' in summary.lower():
            result += "**📈 Historical Context:**\n"
            result += f"The summary indicates historical performance data is available.\n"
            result += f"Compare current results with historical trends for context.\n\n"
        
        # Add guidance for next steps
        if failures:
            result += "**🔍 Recommended Next Steps:**\n"
            result += "1. Analyze failing jobs using the job summary tool\n"
            result += "2. Look for common failure patterns across failed jobs\n"
            result += "3. Check for known incidents that might correlate with failures\n"
            result += "4. Compare failure reasons to understand if they're related\n"
        
        return result
