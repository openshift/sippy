"""
Tool for parsing JUnit XML files and extracting test failures and flakes.
"""

import logging
import xml.etree.ElementTree as ET
from typing import Any, Dict, List, Optional, Type
from pydantic import Field
import httpx
import yaml

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class JUnitParserTool(SippyBaseTool):
    """Tool for parsing JUnit XML files to extract test failures and flakes."""
    
    name: str = "parse_junit_xml"
    description: str = "Parse JUnit XML file from URL to get test failures and flakes. Takes junit_xml_url as required parameter and optional test_name parameter."
    
    class JUnitParserInput(SippyToolInput):
        junit_xml_url: str = Field(description="URL to the JUnit XML file")
        test_name: Optional[str] = Field(default=None, description="Optional specific test name to get results for")
    
    args_schema: Type[SippyToolInput] = JUnitParserInput
    
    def _run(self, junit_xml_url: str, test_name: Optional[str] = None) -> str:
        """Parse JUnit XML file and extract test failures and flakes."""
        try:
            # Handle case where agent passes JSON string instead of parsed parameters
            if junit_xml_url.startswith('{') and junit_xml_url.endswith('}'):
                try:
                    import json
                    parsed = json.loads(junit_xml_url)
                    if 'junit_xml_url' in parsed:
                        junit_xml_url = parsed['junit_xml_url']
                        if 'test_name' in parsed:
                            test_name = parsed['test_name']
                except json.JSONDecodeError:
                    return f"Error: Received malformed JSON input: {junit_xml_url}"

            # Fetch the XML content
            logger.info(f"Fetching JUnit XML from: {junit_xml_url}")
            
            with httpx.Client(timeout=60.0) as client:
                response = client.get(junit_xml_url)
                response.raise_for_status()
                
                xml_content = response.text
                
            # Parse the XML
            try:
                root = ET.fromstring(xml_content)
            except ET.ParseError as e:
                logger.error(f"XML parse error: {e}")
                return f"Error: Invalid XML format - {str(e)}"
            
            # Check if this is an aggregated JUnit file first
            aggregated_results = self._extract_aggregated_yaml_from_xml(root)
            if aggregated_results:
                return self._format_aggregated_results(aggregated_results)

            # Extract regular test results
            test_results = self._extract_test_results(root)

            # Check for underlying job links in regular JUnit
            underlying_jobs = self._extract_underlying_job_links(xml_content)
            
            # Process results based on requirements
            if test_name:
                # Return all results for the specific test (no overall size limit for specific tests)
                filtered_results = [result for result in test_results if result['name'] == test_name]
                if not filtered_results:
                    return f"No test results found for test name: {test_name}"
                return self._format_test_results(filtered_results, test_name, underlying_jobs)
            else:
                # Return only failures and flakes, limit to 25 but respect 150KB overall limit
                failures_and_flakes = self._identify_failures_and_flakes(test_results)

                # Format results while respecting the 150KB overall limit
                result_text, actual_count, total_count = self._format_test_results_with_limit(failures_and_flakes, underlying_jobs, max_size_kb=150)

                if actual_count < total_count:
                    result_text += f"\n\n**Note:** Results truncated to {actual_count} entries due to size limit. Total failures/flakes found: {total_count}"
                elif len(failures_and_flakes) > 25:
                    result_text += f"\n\n**Note:** Results truncated to first 25 entries. Total failures/flakes found: {len(failures_and_flakes)}"

                return result_text
                
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error fetching JUnit XML: {e}")
            return f"Error: HTTP {e.response.status_code} - Failed to fetch XML from {junit_xml_url}"
        except httpx.RequestError as e:
            logger.error(f"Request error fetching JUnit XML: {e}")
            return f"Error: Failed to connect to {junit_xml_url} - {str(e)}"
        except Exception as e:
            logger.error(f"Unexpected error parsing JUnit XML: {e}")
            return f"Error: Unexpected error - {str(e)}"
    
    def _extract_test_results(self, root: ET.Element) -> List[Dict[str, Any]]:
        """Extract all test results from the JUnit XML."""
        test_results = []
        
        # Handle different JUnit XML structures
        # Look for testcase elements at various levels
        testcases = []
        
        # Direct testcase children
        testcases.extend(root.findall('.//testcase'))
        
        for testcase in testcases:
            test_name = testcase.get('name', 'Unknown')
            classname = testcase.get('classname', '')
            time_str = testcase.get('time', '0')
            
            # Parse duration
            try:
                duration = float(time_str)
            except (ValueError, TypeError):
                duration = 0.0
            
            # Determine test result
            failure = testcase.find('failure')
            error = testcase.find('error')
            skipped = testcase.find('skipped')
            
            if failure is not None:
                status = 'failure'
                output = failure.text or failure.get('message', '')
            elif error is not None:
                status = 'failure'
                output = error.text or error.get('message', '')
            elif skipped is not None:
                status = 'skipped'
                output = skipped.text or skipped.get('message', '')
            else:
                status = 'success'
                output = ''
            
            # Get system-out and system-err if available
            system_out = testcase.find('system-out')
            system_err = testcase.find('system-err')
            
            additional_output = []
            if system_out is not None and system_out.text:
                additional_output.append(f"STDOUT:\n{system_out.text}")
            if system_err is not None and system_err.text:
                additional_output.append(f"STDERR:\n{system_err.text}")
            
            if additional_output:
                if output:
                    output += "\n\n" + "\n\n".join(additional_output)
                else:
                    output = "\n\n".join(additional_output)
            
            # Truncate output to first 15KB
            if len(output) > 15360:  # 15KB
                output = output[:15360] + "\n... [output truncated to 15KB]"
            
            test_results.append({
                'name': test_name,
                'classname': classname,
                'full_name': f"{classname}.{test_name}" if classname else test_name,
                'duration': duration,
                'status': status,
                'output': output
            })
        
        return test_results

    def _extract_aggregated_yaml_from_xml(self, root: ET.Element) -> Optional[List[Dict[str, Any]]]:
        """Extract aggregated YAML data from JUnit XML system-out sections, but only for failed tests."""
        import yaml

        aggregated_tests = []

        # Look for testcase elements with system-out containing YAML
        testcases = root.findall('.//testcase')

        for testcase in testcases:
            # Only process testcases that have actual JUnit failures (indicated by <failure> element)
            failure_element = testcase.find('failure')
            if failure_element is None:
                # This test passed according to JUnit, skip it
                continue

            system_out = testcase.find('system-out')
            if system_out is not None and system_out.text:
                try:
                    # Try to parse the system-out content as YAML
                    yaml_content = system_out.text.strip()
                    if yaml_content and ('passes:' in yaml_content or 'failures:' in yaml_content):
                        yaml_data = yaml.safe_load(yaml_content)
                        if isinstance(yaml_data, dict):
                            # Add test case name and failure info to the YAML data
                            yaml_data['testcase_name'] = testcase.get('name', 'Unknown')
                            yaml_data['junit_failure_message'] = failure_element.get('message', 'No failure message')
                            aggregated_tests.append(yaml_data)
                except yaml.YAMLError:
                    # Not valid YAML, continue
                    continue

        return aggregated_tests if aggregated_tests else None

    def _format_aggregated_results(self, aggregated_tests: List[Dict[str, Any]]) -> str:
        """Format aggregated test results for display - only show tests that actually failed according to JUnit."""
        result = "**🔄 Aggregated Test Results - Failed Tests Only**\n\n"

        if not aggregated_tests:
            result += "**✅ All aggregated tests passed!** No JUnit failures detected.\n"
            result += "This means all statistical aggregations met their required pass thresholds.\n"
            return result

        total_failed_tests = len(aggregated_tests)
        # Limit to 25 aggregated tests maximum
        max_tests_to_show = 25
        tests_to_show = aggregated_tests[:max_tests_to_show]

        result += f"**❌ Failed Aggregated Tests ({total_failed_tests} total"
        if total_failed_tests > max_tests_to_show:
            result += f", showing first {max_tests_to_show}"
        result += "):**\n"
        result += "These tests failed their statistical aggregation requirements.\n\n"

        for i, test_data in enumerate(tests_to_show, 1):
            testcase_name = test_data.get('testcase_name', f'Test {i}')
            testsuitename = test_data.get('testsuitename', 'Unknown')
            summary = test_data.get('summary', 'No summary available')
            junit_failure = test_data.get('junit_failure_message', 'No failure message')

            result += f"**{i}. {testcase_name}**\n"
            result += f"**Suite:** {testsuitename}\n"
            result += f"**Summary:** {summary}\n"
            result += f"**JUnit Failure:** {junit_failure}\n\n"

            # Process passes, failures, and skips from the underlying jobs
            passes = test_data.get('passes', [])
            failures = test_data.get('failures', [])
            skips = test_data.get('skips', [])

            # Deduplicate all job lists for accurate counts
            unique_passes = []
            seen_pass_ids = set()
            for job in passes:
                job_id = job.get('jobrunid', 'Unknown')
                if job_id not in seen_pass_ids:
                    unique_passes.append(job)
                    seen_pass_ids.add(job_id)

            unique_failures = []
            seen_fail_ids = set()
            for job in failures:
                job_id = job.get('jobrunid', 'Unknown')
                if job_id not in seen_fail_ids:
                    unique_failures.append(job)
                    seen_fail_ids.add(job_id)

            unique_skips = []
            seen_skip_ids = set()
            for job in skips:
                job_id = job.get('jobrunid', 'Unknown')
                if job_id not in seen_skip_ids:
                    unique_skips.append(job)
                    seen_skip_ids.add(job_id)

            # Show underlying job breakdown with unique counts
            total_unique_jobs = len(unique_passes) + len(unique_failures) + len(unique_skips)
            if total_unique_jobs > 0:
                pass_rate = (len(unique_passes) / total_unique_jobs) * 100
                result += f"**📊 Underlying Job Breakdown:**\n"
                result += f"- Total unique underlying jobs: {total_unique_jobs}\n"
                result += f"- Unique passing jobs: {len(unique_passes)}\n"
                result += f"- Unique failing jobs: {len(unique_failures)}\n"
                result += f"- Unique skipped jobs: {len(unique_skips)}\n"
                result += f"- Actual pass rate: {pass_rate:.1f}%\n\n"

            # Show some example failing jobs if they exist (using already deduplicated list)
            if unique_failures:
                result += f"**❌ Example Failing Jobs (showing up to 3 unique):**\n"
                for j, job in enumerate(unique_failures[:3], 1):
                    job_id = job.get('jobrunid', 'Unknown')
                    human_url = job.get('humanurl', 'No URL')
                    result += f"  {j}. Job ID {job_id}: {human_url}\n"

                if len(unique_failures) > 3:
                    result += f"  ... and {len(unique_failures) - 3} more unique failing jobs\n"
                result += "\n"

            # Show some example passing jobs for context (using already deduplicated list)
            if unique_passes:
                result += f"**✅ Example Passing Jobs (showing up to 2 unique):**\n"
                for j, job in enumerate(unique_passes[:2], 1):
                    job_id = job.get('jobrunid', 'Unknown')
                    human_url = job.get('humanurl', 'No URL')
                    result += f"  {j}. Job ID {job_id}: {human_url}\n"

                if len(unique_passes) > 2:
                    result += f"  ... and {len(unique_passes) - 2} more unique passing jobs\n"
                result += "\n"

            result += "---\n\n"

        # Add note if there are more tests than shown
        if total_failed_tests > max_tests_to_show:
            remaining = total_failed_tests - max_tests_to_show
            result += f"**Note:** Results truncated to first {max_tests_to_show} entries. Total failed aggregated tests found: {total_failed_tests} ({remaining} more not shown)\n\n"

        # Add guidance for next steps
        if aggregated_tests:
            result += "**🔍 Recommended Next Steps:**\n"
            result += "1. Focus on the JUnit failure messages above - these explain why the aggregation failed\n"
            result += "2. Use the job summary tool on example failing job IDs to understand specific failure modes\n"
            result += "3. Look for patterns: are failures consistent across jobs or sporadic?\n"
            result += "4. Check if the failure rate exceeds historical thresholds mentioned in summaries\n"
            result += "5. Only analyze individual underlying jobs if specifically requested for deep analysis\n"

        return result

    def _extract_underlying_job_links(self, xml_content: str) -> List[Dict[str, str]]:
        """Extract underlying job links from aggregated JUnit XML content."""
        underlying_jobs = []

        # Look for URLs in the XML content that point to underlying jobs
        import re

        # Pattern to match prow job URLs in the XML content
        url_pattern = r'https://prow\.ci\.openshift\.org/view/gs/[^\s<>"\']+/(\d+)'

        # Find all URLs and extract job IDs
        urls = re.findall(url_pattern, xml_content)

        # Also look for job IDs in failure messages or test output
        # Pattern for job IDs that might be mentioned in test failures
        job_id_pattern = r'job[_\s]*(?:id|run)[_\s]*:?\s*(\d{10,})'
        job_ids_from_text = re.findall(job_id_pattern, xml_content, re.IGNORECASE)

        # Combine and deduplicate job IDs
        all_job_ids = set(urls + job_ids_from_text)

        for job_id in all_job_ids:
            underlying_jobs.append({
                'job_id': job_id,
                'url': f"https://prow.ci.openshift.org/view/gs/test-platform-results/logs/{job_id}"
            })

        # Also look for specific patterns in aggregated test output that might contain links
        # Pattern to find "PASSING" and "FAILING" job links in test output
        link_pattern = r'(PASSING|FAILING)\s+jobs?[:\s]*([^\s<>"\']+)'
        links = re.findall(link_pattern, xml_content, re.IGNORECASE)

        for status, url in links:
            if 'prow.ci.openshift.org' in url or url.startswith('http'):
                # Extract job ID from URL if possible
                job_id_match = re.search(r'/(\d{10,})/?$', url)
                if job_id_match:
                    job_id = job_id_match.group(1)
                    underlying_jobs.append({
                        'job_id': job_id,
                        'url': url,
                        'status': status.upper()
                    })

        return underlying_jobs

    def _identify_failures_and_flakes(self, test_results: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Identify failures and flakes from test results."""
        # Group tests by full name to identify flakes
        test_groups = {}
        for result in test_results:
            full_name = result['full_name']
            if full_name not in test_groups:
                test_groups[full_name] = []
            test_groups[full_name].append(result)
        
        failures_and_flakes = []
        
        for full_name, results in test_groups.items():
            if len(results) == 1:
                # Single test run
                result = results[0]
                if result['status'] in ['failure', 'error']:
                    failures_and_flakes.append(result)
            else:
                # Multiple test runs - check for flakes
                statuses = [r['status'] for r in results]
                success_count = statuses.count('success')
                failure_count = len([s for s in statuses if s in ['failure', 'error']])
                
                if success_count > 0 and failure_count > 0:
                    # This is a flake
                    # Create a combined result
                    total_duration = sum(r['duration'] for r in results)
                    combined_output = []
                    
                    for i, result in enumerate(results):
                        combined_output.append(f"Run {i+1} ({result['status']}):")
                        if result['output']:
                            combined_output.append(result['output'])
                        combined_output.append("")
                    
                    output_text = "\n".join(combined_output)
                    if len(output_text) > 15360:  # 15KB
                        output_text = output_text[:15360] + "\n... [output truncated to 15KB]"
                    
                    flake_result = {
                        'name': results[0]['name'],
                        'classname': results[0]['classname'],
                        'full_name': full_name,
                        'duration': total_duration,
                        'status': 'flake',
                        'success_count': success_count,
                        'failure_count': failure_count,
                        'output': output_text
                    }
                    failures_and_flakes.append(flake_result)
                elif failure_count > 0:
                    # All failures - add the first failure
                    failure_result = next(r for r in results if r['status'] in ['failure', 'error'])
                    failures_and_flakes.append(failure_result)
        
        return failures_and_flakes
    
    def _format_test_results(self, results: List[Dict[str, Any]], specific_test: Optional[str] = None, underlying_jobs: Optional[List[Dict[str, str]]] = None) -> str:
        """Format test results for display."""
        if not results:
            if specific_test:
                return f"No results found for test: {specific_test}"
            else:
                return "No test failures or flakes found in the JUnit XML file."

        if specific_test:
            header = f"**Test Results for: {specific_test}**\n\n"
        else:
            header = f"**JUnit Test Failures and Flakes**\n\n"
            header += f"Found {len(results)} test failures/flakes:\n\n"

        formatted_results = []
        
        for i, result in enumerate(results, 1):
            test_info = f"**{i}. {result['name']}**\n"
            
            if result['classname']:
                test_info += f"   **Class:** {result['classname']}\n"
            
            test_info += f"   **Duration:** {result['duration']:.2f}s\n"
            
            if result['status'] == 'flake':
                test_info += f"   **Result:** FLAKE ({result['success_count']} successes, {result['failure_count']} failures)\n"
            else:
                test_info += f"   **Result:** {result['status'].upper()}\n"
            
            if result['output']:
                test_info += f"   **Output:**\n```\n{result['output']}\n```\n"
            
            formatted_results.append(test_info)
        
        result = header + "\n".join(formatted_results)

        # Add underlying jobs information if this is an aggregated job
        if underlying_jobs:
            result += "\n\n🔄 **AGGREGATED JOB - UNDERLYING JOBS DETECTED:**\n"
            result += "This JUnit XML contains results from multiple underlying job runs.\n\n"

            if len(underlying_jobs) > 0:
                result += "**Underlying Job Links:**\n"
                for i, job in enumerate(underlying_jobs[:10], 1):  # Limit to first 10
                    status_info = f" ({job['status']})" if 'status' in job else ""
                    result += f"{i}. Job ID {job['job_id']}{status_info}: {job['url']}\n"

                if len(underlying_jobs) > 10:
                    result += f"... and {len(underlying_jobs) - 10} more underlying jobs\n"

                result += "\n💡 **For deep analysis:** Use the job summary tool on individual job IDs above to analyze specific failures.\n"

        return result

    def _format_test_results_with_limit(self, results: List[Dict[str, Any]], underlying_jobs: Optional[List[Dict[str, str]]] = None, max_size_kb: int = 150) -> tuple[str, int, int]:
        """Format test results while respecting overall size limit.

        Returns:
            tuple: (formatted_text, actual_count, total_count)
        """
        if not results:
            return "No test failures or flakes found in the JUnit XML file.", 0, 0

        max_size_bytes = max_size_kb * 1024
        total_count = len(results)

        # Limit to 25 results initially
        limited_results = results[:25]

        header = f"**JUnit Test Failures and Flakes**\n\n"
        header += f"Found {len(limited_results)} test failures/flakes:\n\n"

        formatted_results = []
        current_size = len(header.encode('utf-8'))
        actual_count = 0

        for i, result in enumerate(limited_results, 1):
            test_info = f"**{i}. {result['name']}**\n"

            if result['classname']:
                test_info += f"   **Class:** {result['classname']}\n"

            test_info += f"   **Duration:** {result['duration']:.2f}s\n"

            if result['status'] == 'flake':
                test_info += f"   **Result:** FLAKE ({result['success_count']} successes, {result['failure_count']} failures)\n"
            else:
                test_info += f"   **Result:** {result['status'].upper()}\n"

            if result['output']:
                test_info += f"   **Output:**\n```\n{result['output']}\n```\n"

            test_info += "\n"

            # Check if adding this test would exceed the size limit
            test_size = len(test_info.encode('utf-8'))
            if current_size + test_size > max_size_bytes and actual_count > 0:
                # Stop adding tests if we would exceed the limit
                break

            formatted_results.append(test_info)
            current_size += test_size
            actual_count += 1

        result = header + "\n".join(formatted_results)

        # Add underlying jobs information if this is an aggregated job
        if underlying_jobs:
            result += "\n\n🔄 **AGGREGATED JOB - UNDERLYING JOBS DETECTED:**\n"
            result += "This JUnit XML contains results from multiple underlying job runs.\n\n"

            if len(underlying_jobs) > 0:
                result += "**Underlying Job Links:**\n"
                for i, job in enumerate(underlying_jobs[:10], 1):  # Limit to first 10
                    status_info = f" ({job['status']})" if 'status' in job else ""
                    result += f"{i}. Job ID {job['job_id']}{status_info}: {job['url']}\n"

                if len(underlying_jobs) > 10:
                    result += f"... and {len(underlying_jobs) - 10} more underlying jobs\n"

                result += "\n💡 **For deep analysis:** Use the job summary tool on individual job IDs above to analyze specific failures.\n"

        return result, actual_count, total_count
