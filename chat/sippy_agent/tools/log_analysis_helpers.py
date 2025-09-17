"""
Helper functions for analyzing log patterns and errors.
"""

from typing import Any, Dict, List


def analyze_error_patterns(matches: list) -> str:
    """Analyze error patterns and provide insights."""
    analysis = "**🔍 Error Analysis:**\n"
    
    # Categorize errors
    operator_errors = []
    installation_errors = []
    network_errors = []
    timeout_errors = []
    step_failures = []
    entrypoint_errors = []
    registry_errors = []
    other_errors = []
    
    for match_obj in matches:
        match_text = match_obj.get("match", str(match_obj)).lower()
        
        # Check for cluster operator issues first (most important for installation failures)
        if any(keyword in match_text for keyword in ["operator", "clusteroperator", "degraded", "progressing", "available"]):
            operator_errors.append(match_obj)
        elif any(keyword in match_text for keyword in ["failed to install", "installation", "cluster install"]):
            installation_errors.append(match_obj)
        elif any(keyword in match_text for keyword in ["network", "dns", "connectivity", "cni"]):
            network_errors.append(match_obj)
        elif "registry" in match_text and ("503" in match_text or "unavailable" in match_text):
            registry_errors.append(match_obj)
        elif "timeout" in match_text or "timed out" in match_text:
            timeout_errors.append(match_obj)
        elif "step" in match_text and "failed" in match_text:
            step_failures.append(match_obj)
        elif "entrypoint" in match_text:
            entrypoint_errors.append(match_obj)
        else:
            other_errors.append(match_obj)
    
    # Provide analysis based on error types (prioritize installation/operator issues)
    if operator_errors:
        analysis += f"\n🔧 **Cluster Operator Issues ({len(operator_errors)} occurrences):**\n"
        # Extract operator details
        for error in operator_errors[:3]:  # Show first 3
            error_text = error.get("match", "")
            if "degraded" in error_text.lower():
                analysis += f"- Operator in degraded state: {error_text[:120]}...\n"
            elif "progressing" in error_text.lower():
                analysis += f"- Operator stuck progressing: {error_text[:120]}...\n"
            elif "available" in error_text.lower():
                analysis += f"- Operator availability issue: {error_text[:120]}...\n"
            else:
                analysis += f"- Operator issue: {error_text[:120]}...\n"
        analysis += "💡 *Check cluster operator status - this is likely the root cause of installation failure*\n"
    
    if installation_errors:
        analysis += f"\n🏗️ **Installation Issues ({len(installation_errors)} occurrences):**\n"
        for error in installation_errors[:2]:  # Show first 2
            error_text = error.get("match", "")
            analysis += f"- Installation failure: {error_text[:120]}...\n"
        analysis += "💡 *Focus on installation logs and cluster operator health*\n"
    
    if network_errors:
        analysis += f"\n🌐 **Network Issues ({len(network_errors)} occurrences):**\n"
        for error in network_errors[:2]:  # Show first 2
            error_text = error.get("match", "")
            analysis += f"- Network problem: {error_text[:120]}...\n"
        analysis += "💡 *Check network operator status and cluster networking configuration*\n"
    
    if registry_errors:
        analysis += f"\n📦 **Registry Issues ({len(registry_errors)} occurrences):**\n"
        # Extract registry details
        for error in registry_errors[:2]:  # Show first 2
            error_text = error.get("match", "")
            if "registry.build11.ci.openshift.org" in error_text:
                analysis += "- Problems with external registry (503 Service Unavailable)\n"
            elif "registry" in error_text:
                analysis += f"- Registry connectivity issue: {error_text[:100]}...\n"
        analysis += "💡 *This suggests external registry infrastructure issues*\n"
    
    if step_failures:
        analysis += f"\n⚠️ **Step Failures ({len(step_failures)} occurrences):**\n"
        failed_steps = set()
        for error in step_failures:
            error_text = error.get("match", "")
            if "step " in error_text.lower():
                # Extract step name
                import re
                step_match = re.search(r'step ([a-zA-Z0-9-_]+)', error_text.lower())
                if step_match:
                    failed_steps.add(step_match.group(1))
        
        for step in list(failed_steps)[:3]:  # Show first 3 unique steps
            analysis += f"- {step}\n"
        analysis += "💡 *Multiple pipeline steps failed, check for underlying issues*\n"
    
    if entrypoint_errors:
        analysis += f"\n🔄 **Process Issues ({len(entrypoint_errors)} occurrences):**\n"
        analysis += "- Test process execution failures\n"
        analysis += "- Process termination/interruption\n"
        analysis += "💡 *These are likely secondary failures caused by the primary issue*\n"
    
    if timeout_errors:
        analysis += f"\n⏱️ **Timeout Issues ({len(timeout_errors)} occurrences):**\n"
        analysis += "- Operations timed out\n"
    
    # Provide overall assessment
    analysis += f"\n**🎯 Root Cause Assessment:**\n"
    if operator_errors:
        analysis += "Primary issue appears to be cluster operator problems. "
        analysis += "One or more operators are in a degraded or unavailable state, preventing successful installation.\n"
        analysis += "💡 *Focus: Check specific operator status and configuration rather than infrastructure incidents*\n"
    elif installation_errors and not operator_errors:
        analysis += "Installation failed but no specific operator errors detected. "
        analysis += "Check installation logs for configuration or resource issues.\n"
        analysis += "💡 *Focus: Review installation process and cluster requirements*\n"
    elif network_errors:
        analysis += "Network-related issues detected. This could affect cluster communication and operator functionality.\n"
        analysis += "💡 *Check: Network operator status and cluster networking configuration*\n"
    elif registry_errors:
        analysis += "External registry connectivity problems detected. "
        analysis += "This may affect image pulls but is likely not the root cause of installation failures.\n"
        analysis += "💡 *Check: Known registry incidents, but focus on installation-specific issues*\n"
    elif timeout_errors and step_failures:
        analysis += "Multiple timeouts and step failures suggest resource constraints or infrastructure issues.\n"
        analysis += "💡 *Check: Infrastructure capacity and known infrastructure incidents*\n"
    elif step_failures:
        analysis += "Multiple step failures suggest a systematic issue in the pipeline.\n"
        # Check if these are test failures vs infrastructure failures
        test_related = any("test" in error.get("match", "").lower() for error in step_failures)
        if test_related:
            analysis += "💡 *Note: These appear to be test-related failures, likely unrelated to infrastructure incidents*\n"
        else:
            analysis += "💡 *Check: Pipeline configuration and known infrastructure incidents*\n"
    else:
        analysis += "Mixed error patterns - requires deeper investigation of specific failure types.\n"
        analysis += "💡 *Note: Analyze specific error types before checking for related incidents*\n"
    
    return analysis


def format_log_analysis(data: Any, job_id: str, path_glob: str, regex: str) -> str:
    """Format the log analysis results for display."""
    if not data:
        return f"No artifacts found matching pattern '{path_glob}' with regex '{regex}' for job {job_id}"
    
    result = f"**Log Analysis Results**\n\n"
    result += f"**Job Run ID:** {job_id}\n"
    result += f"**Path Pattern:** {path_glob}\n"
    result += f"**Search Pattern:** {regex}\n\n"
    
    # Handle the new Sippy API response format
    if isinstance(data, dict) and "job_runs" in data:
        job_runs = data.get("job_runs", [])
        if not job_runs:
            result += "**Results:** No job runs found\n"
            return result
        
        for job_run in job_runs:
            artifacts = job_run.get("artifacts", [])
            if not artifacts:
                result += "**Results:** No artifacts found\n"
                continue
            
            result += f"**Found {len(artifacts)} matching artifacts:**\n\n"
            
            # Analyze each artifact
            for artifact in artifacts:
                artifact_path = artifact.get("artifact_path", "unknown")
                artifact_url = artifact.get("artifact_url", "")
                matched_content = artifact.get("matched_content", {})
                
                result += f"**📁 {artifact_path}**\n"
                if artifact_url:
                    result += f"🔗 [View full log]({artifact_url})\n\n"
                
                # Process line matches
                line_matches = matched_content.get("line_matches", {})
                matches = line_matches.get("matches", [])
                
                if matches:
                    result += f"**Found {len(matches)} error/failure patterns:**\n\n"
                    
                    # Analyze and categorize the errors
                    analysis = analyze_error_patterns(matches)
                    result += analysis
                    
                    # Show first few raw matches for reference
                    result += "\n**Raw Error Lines:**\n"
                    for i, match_obj in enumerate(matches[:5], 1):
                        match_text = match_obj.get("match", str(match_obj))
                        # Clean up the match text
                        clean_match = match_text.strip().replace('\n', ' ')
                        if len(clean_match) > 200:
                            clean_match = clean_match[:200] + "..."
                        result += f"{i}. {clean_match}\n"
                    
                    if len(matches) > 5:
                        result += f"... and {len(matches) - 5} more error lines\n"
                    
                    if line_matches.get("truncated"):
                        result += "\n⚠️ *Results were truncated - there may be more errors*\n"
                else:
                    result += "No error patterns found in this artifact\n"
                
                result += "\n"
    else:
        # Fallback for other response formats
        result += f"**Results:**\n{str(data)[:500]}...\n"
    
    return result
