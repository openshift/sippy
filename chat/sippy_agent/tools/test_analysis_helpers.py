"""
Helper functions for analyzing test failures.
"""

from typing import Dict


def analyze_test_failures(test_failures: Dict[str, str]) -> str:
    """Analyze test failure patterns and provide insights."""
    if not test_failures:
        return ""
    
    # Categorize tests by their sig (special interest group)
    categories = {}
    error_patterns = {}
    
    for test_name, failure_msg in test_failures.items():
        # Extract category from test name
        category = extract_test_category(test_name)
        if category:
            categories[category] = categories.get(category, 0) + 1
        
        # Look for common error patterns
        failure_lower = failure_msg.lower()
        if "timeout" in failure_lower or "timed out" in failure_lower:
            error_patterns["timeout"] = error_patterns.get("timeout", 0) + 1
        elif "connection" in failure_lower or "network" in failure_lower:
            error_patterns["network"] = error_patterns.get("network", 0) + 1
        elif "permission" in failure_lower or "forbidden" in failure_lower:
            error_patterns["permissions"] = error_patterns.get("permissions", 0) + 1
        elif "not found" in failure_lower or "404" in failure_lower:
            error_patterns["missing_resources"] = error_patterns.get("missing_resources", 0) + 1
        elif "pod" in failure_lower and ("crash" in failure_lower or "failed" in failure_lower):
            error_patterns["pod_failures"] = error_patterns.get("pod_failures", 0) + 1
    
    analysis = ""
    
    # Analyze by category
    if categories:
        analysis += "**Test Categories Affected:**\n"
        for category, count in sorted(categories.items(), key=lambda x: x[1], reverse=True):
            analysis += f"- {category}: {count} test(s)\n"
        analysis += "\n"
    
    # Analyze by error patterns
    if error_patterns:
        analysis += "**Common Error Patterns:**\n"
        for pattern, count in sorted(error_patterns.items(), key=lambda x: x[1], reverse=True):
            analysis += f"- {pattern.replace('_', ' ').title()}: {count} occurrence(s)\n"
        analysis += "\n"
    
    # Provide insights based on patterns
    insights = generate_test_insights(categories, error_patterns)
    if insights:
        analysis += f"**💡 Insights:**\n{insights}\n"
    
    return analysis


def extract_test_category(test_name: str) -> str:
    """Extract the test category/sig from test name."""
    import re
    
    # Look for [sig-xxx] pattern
    sig_match = re.search(r'\[sig-([^\]]+)\]', test_name)
    if sig_match:
        return f"sig-{sig_match.group(1)}"
    
    # Look for other common patterns
    if "[Feature:" in test_name:
        feature_match = re.search(r'\[Feature:([^\]]+)\]', test_name)
        if feature_match:
            return f"Feature: {feature_match.group(1)}"
    
    if "[Suite:" in test_name:
        suite_match = re.search(r'\[Suite:([^\]]+)\]', test_name)
        if suite_match:
            return f"Suite: {suite_match.group(1)}"
    
    return "Unknown"


def clean_failure_message(failure_msg: str) -> str:
    """Clean and format failure message for better readability."""
    if not failure_msg:
        return "No error message available"
    
    # Remove excessive whitespace and newlines
    clean_msg = ' '.join(failure_msg.split())
    
    # Extract key error information
    import re
    
    # Look for common error patterns and extract the most relevant part
    error_patterns = [
        r'Error: ([^\\n]+)',
        r'error: ([^\\n]+)',
        r'FAIL: ([^\\n]+)',
        r'failed: ([^\\n]+)',
        r'Expected[^,]+, got ([^\\n]+)',
        r'timeout: ([^\\n]+)',
    ]
    
    for pattern in error_patterns:
        match = re.search(pattern, clean_msg, re.IGNORECASE)
        if match:
            key_error = match.group(1).strip()
            if len(key_error) < 150:
                return key_error
    
    # If no specific pattern found, truncate intelligently
    if len(clean_msg) > 200:
        # Try to find a good breaking point
        truncate_at = 200
        space_pos = clean_msg.rfind(' ', 0, truncate_at)
        if space_pos > 150:
            truncate_at = space_pos
        return clean_msg[:truncate_at] + "..."
    
    return clean_msg


def generate_test_insights(categories: Dict[str, int], error_patterns: Dict[str, int]) -> str:
    """Generate insights based on test failure patterns."""
    insights = []
    
    # Category-based insights
    if "sig-network" in categories:
        insights.append("Network-related tests are failing, suggesting potential networking configuration or connectivity issues.")
    
    if "sig-storage" in categories:
        insights.append("Storage tests are failing, indicating possible persistent volume or storage class issues.")
    
    if "sig-auth" in categories:
        insights.append("Authentication/authorization tests are failing, suggesting RBAC or security configuration problems.")
    
    if "sig-api-machinery" in categories:
        insights.append("API machinery tests are failing, indicating potential Kubernetes API server or etcd issues.")
    
    # Error pattern insights
    if error_patterns.get("timeout", 0) > 0:
        insights.append("Multiple timeout errors suggest resource constraints or slow operations in the test environment.")
    
    if error_patterns.get("network", 0) > 0:
        insights.append("Network connectivity errors indicate potential DNS, routing, or firewall issues.")
    
    if error_patterns.get("permissions", 0) > 0:
        insights.append("Permission errors suggest RBAC configuration issues or service account problems.")
    
    if error_patterns.get("pod_failures", 0) > 0:
        insights.append("Pod failures indicate potential resource limits, image pull issues, or application configuration problems.")
    
    # Multi-category insights
    if len(categories) > 3:
        insights.append("Multiple test categories are affected, suggesting a broader infrastructure or configuration issue.")
    
    return "\n".join(f"- {insight}" for insight in insights)
