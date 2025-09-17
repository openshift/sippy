"""
Core Re-Act agent implementation for Sippy.
"""

import logging
import re
from typing import List, Optional, Union, Dict, Any, Callable
from langchain.agents import AgentExecutor, create_react_agent
from langchain.prompts import PromptTemplate
from langchain_openai import ChatOpenAI
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain.tools import BaseTool
from langchain.callbacks.base import BaseCallbackHandler
from langchain.schema import AgentAction, AgentFinish, LLMResult

from .config import Config
from .tools import (
    ExampleTool,
    SippyJobAnalysisTool,
    SippyTestFailureTool,
    SippyProwJobSummaryTool,
    SippyLogAnalyzerTool,
    SippyJiraIncidentTool,
    SippyReleasePayloadTool,
    SippyPayloadDetailsTool,
    SippyReleasesTool,
    JUnitParserTool,
    AggregatedJobAnalyzerTool,
    AggregatedYAMLParserTool
)

logger = logging.getLogger(__name__)


class StreamingThinkingHandler(BaseCallbackHandler):
    """Callback handler to stream thinking process in real-time."""

    def __init__(self, thinking_callback: Optional[Callable[[str, str, str, str], None]] = None):
        """Initialize with optional callback for streaming thoughts."""
        self.thinking_callback = thinking_callback
        self.step_count = 0

    def on_agent_action(self, action: AgentAction, **kwargs) -> None:
        """Called when agent takes an action."""
        if self.thinking_callback:
            self.step_count += 1

            # Extract thought from the action log
            thought = self._extract_thought_from_log(action.log)
            action_name = action.tool
            action_input = str(action.tool_input)

            # Skip if this is an error/exception action
            if action_name in ['_Exception', 'Invalid', 'Error']:
                return

            # Stream the thinking step
            self.thinking_callback(thought, action_name, action_input, "")

    def on_tool_end(self, output: str, **kwargs) -> None:
        """Called when a tool finishes."""
        if self.thinking_callback:
            # Skip error outputs
            if "Invalid" in output or "Error" in output or "_Exception" in output:
                return
            # Stream the observation
            self.thinking_callback("", "", "", output)

    def _extract_thought_from_log(self, log: str) -> str:
        """Extract the thought portion from the action log."""
        if not log:
            return "Processing..."

        # Look for "Thought:" pattern in the log
        thought_match = re.search(r'Thought:\s*(.*?)(?=\nAction:|Action:|$)', log, re.DOTALL | re.IGNORECASE)
        if thought_match:
            return thought_match.group(1).strip()

        # Look for reasoning before Action:
        action_split = log.split('Action:', 1)
        if len(action_split) > 1:
            potential_thought = action_split[0].strip()
            if potential_thought and not potential_thought.startswith('Action'):
                return potential_thought

        # If no explicit thought found, try to extract reasoning from the beginning
        lines = log.split('\n')
        for line in lines:
            line = line.strip()
            if line and not line.startswith('Action:') and not line.startswith('Action Input:'):
                return line

        return "Analyzing..."


class TokenCountingHandler(BaseCallbackHandler):
    """Callback handler to count tokens used in LLM calls."""

    def __init__(self):
        """Initialize token counter."""
        self.total_tokens = 0
        self.prompt_tokens = 0
        self.completion_tokens = 0
        self.call_count = 0

    def on_llm_end(self, response: LLMResult, **kwargs) -> None:
        """Called when LLM finishes generating."""
        self.call_count += 1

        # Try to extract token usage from response
        if hasattr(response, 'llm_output') and response.llm_output:
            token_usage = response.llm_output.get('token_usage', {})
            if token_usage:
                self.total_tokens += token_usage.get('total_tokens', 0)
                self.prompt_tokens += token_usage.get('prompt_tokens', 0)
                self.completion_tokens += token_usage.get('completion_tokens', 0)

                logger.info(f"LLM Call {self.call_count}: "
                           f"Prompt: {token_usage.get('prompt_tokens', 0)}, "
                           f"Completion: {token_usage.get('completion_tokens', 0)}, "
                           f"Total: {token_usage.get('total_tokens', 0)}")

        # For Gemini models, try alternative token counting
        elif hasattr(response, 'generations') and response.generations:
            for generation_list in response.generations:
                for generation in generation_list:
                    if hasattr(generation, 'generation_info') and generation.generation_info:
                        usage = generation.generation_info.get('usage_metadata', {})
                        if usage:
                            prompt_tokens = usage.get('prompt_token_count', 0)
                            completion_tokens = usage.get('candidates_token_count', 0)
                            total_tokens = usage.get('total_token_count', prompt_tokens + completion_tokens)

                            self.total_tokens += total_tokens
                            self.prompt_tokens += prompt_tokens
                            self.completion_tokens += completion_tokens

                            logger.info(f"LLM Call {self.call_count} (Gemini): "
                                       f"Prompt: {prompt_tokens}, "
                                       f"Completion: {completion_tokens}, "
                                       f"Total: {total_tokens}")
                            break

    def get_summary(self) -> Dict[str, int]:
        """Get token usage summary."""
        return {
            'total_tokens': self.total_tokens,
            'prompt_tokens': self.prompt_tokens,
            'completion_tokens': self.completion_tokens,
            'call_count': self.call_count
        }

    def reset(self):
        """Reset token counters."""
        self.total_tokens = 0
        self.prompt_tokens = 0
        self.completion_tokens = 0
        self.call_count = 0


class SippyAgent:
    """LangChain Re-Act agent for CI analysis with Sippy."""
    
    def __init__(self, config: Config):
        """Initialize the Sippy agent with configuration."""
        self.config = config
        self.llm = self._create_llm()
        self.tools = self._create_tools()
        self.agent_executor = self._create_agent_executor()
        self.token_counter = TokenCountingHandler()
    
    def _create_llm(self) -> Union[ChatOpenAI, ChatGoogleGenerativeAI]:
        """Create the language model instance."""
        if self.config.verbose:
            logger.info(f"Creating LLM with endpoint: {self.config.llm_endpoint}")
            logger.info(f"Using model: {self.config.model_name}")

        # Use ChatGoogleGenerativeAI for Gemini models
        if self.config.is_gemini_model():
            if not self.config.google_api_key and not self.config.google_credentials_file:
                raise ValueError("Google API key or service account credentials file is required for Gemini models")

            llm_kwargs = {
                "model": self.config.model_name,
                "temperature": self.config.temperature,
            }

            # Use API key if provided, otherwise use service account credentials
            if self.config.google_api_key:
                llm_kwargs["google_api_key"] = self.config.google_api_key
                if self.config.verbose:
                    logger.info(f"Using ChatGoogleGenerativeAI for Gemini model with API key")
            elif self.config.google_credentials_file:
                # Set the environment variable for Google credentials
                import os
                os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = self.config.google_credentials_file
                if self.config.verbose:
                    logger.info(f"Using ChatGoogleGenerativeAI for Gemini model with service account: {self.config.google_credentials_file}")

            return ChatGoogleGenerativeAI(**llm_kwargs)

        # Use ChatOpenAI for OpenAI and Ollama endpoints
        else:
            llm_kwargs = {
                "model": self.config.model_name,
                "temperature": self.config.temperature,
                "base_url": self.config.llm_endpoint,
            }

            # Only add API key if it's provided (needed for OpenAI, not for local endpoints)
            if self.config.openai_api_key:
                llm_kwargs["openai_api_key"] = self.config.openai_api_key
            else:
                # For local endpoints like Ollama, use a dummy key
                llm_kwargs["openai_api_key"] = "dummy-key"

            if self.config.verbose:
                logger.info(f"Using ChatOpenAI with base_url: {self.config.llm_endpoint}")

            return ChatOpenAI(**llm_kwargs)
    
    def _create_tools(self) -> List[BaseTool]:
        """Create the list of tools available to the agent."""
        tools = [
            ExampleTool(),
            SippyJobAnalysisTool(),
            SippyTestFailureTool(),
            SippyProwJobSummaryTool(sippy_api_url=self.config.sippy_api_url),
            SippyLogAnalyzerTool(sippy_api_url=self.config.sippy_api_url),
            SippyJiraIncidentTool(
                mcp_server_url=self.config.sippy_api_url.rstrip('/') + '/mcp/v1/'
            ),
            SippyReleasePayloadTool(),
            SippyPayloadDetailsTool(),
            SippyReleasesTool(sippy_api_url=self.config.sippy_api_url),
            JUnitParserTool(),
            AggregatedJobAnalyzerTool(sippy_api_url=self.config.sippy_api_url),
            AggregatedYAMLParserTool(),
        ]
        
        if self.config.verbose:
            logger.info(f"Created {len(tools)} tools: {[tool.name for tool in tools]}")
        
        return tools
    
    def _create_agent_executor(self) -> AgentExecutor:
        """Create the Re-Act agent executor."""
        # Custom prompt template for Sippy CI analysis
        prompt_template = """You are Sippy AI, an expert assistant for analyzing CI job and test failures.

🚨 CRITICAL EFFICIENCY RULES - READ FIRST:
==========================================
1. If user asks for information available in the job summary, DO NOT search logs! However, if you need additional information consider searching the build logs for errors.
2. READ tool responses carefully - extract information directly before calling more tools
3. Use information you already have instead of making redundant tool calls
4. 🚨 NEVER call the same tool with the same parameters twice! If you already called analyze_job_logs with job ID X and pathGlob Y, use those results!  Same thing for incidents, etc.
5. If a tool call didn't give you what you need, try DIFFERENT parameters, don't repeat the same call, but don't excessively use the tools. Tell the user you don't know if you don't know.
6. 🚨 If a tool fails or gives an error, DO NOT retry it immediately - either try a different tool or provide an answer based on what you know
7. 🚨 For simple questions that don't require tools (like "hello", "hi", "what tools do you have", greetings), answer directly with "Final Answer:" - DO NOT use any actions or tools

You have access to tools that can help you analyze CI jobs, and test failures.

When users ask about CI issues, use the available tools to gather information and provide detailed analysis. Pay attention to
the user's query and ensure you are answering the direction question they gave you.

Example: If the question is answerable by the first tool call, you don't need to continue on.

🚨 GENERAL PRINCIPLE - LOG ANALYSIS:
===================================
- Job summaries often contain sufficient information to answer user questions
- Only analyze logs when the user explicitly asks for log analysis OR when job summaries lack necessary detail
- For questions about "what failed" or "what jobs failed", job summaries are usually sufficient
- For questions about "why did it fail" or "what errors occurred", log analysis may be needed
- Always ask before proceeding to log analysis unless explicitly requested

CI JOB ANALYSIS WORKFLOW:
-------------------------
When analyzing a job failure, follow this conservative workflow:
1. First, use get_prow_job_summary with just the numeric job ID (e.g., 1934795512955801600)
2. Analyze the job summary information, including test failures and basic failure reasons.
3. If the job summary provides sufficient information to answer the user's question, STOP HERE
4. Only proceed to log analysis if:
   - The user explicitly asks for log analysis, OR
   - The job summary doesn't contain enough detail to answer the user's specific question, for example
     test failures are too generic.
5. When analyzing logs: use analyze_job_logs with the numeric job ID
6. Only use check_known_incidents if specific error patterns are found that warrant correlation

AGGREGATED JOB ANALYSIS WORKFLOW:
--------------------------------
For jobs that start with "aggregated-", follow this specific workflow:
1. ALWAYS start with get_prow_job_summary to get the list of failed tests from the job summary
2. Report the failed tests and basic information from the job summary.  If you are investigating a release
payload, consider if any of the test failures are related to the changelog.
3. ONLY proceed to detailed aggregated analysis if the user specifically asks for:
   - "detailed analysis of the aggregated job"
   - "underlying job analysis"
   - "why did the underlying jobs fail"
   - Similar explicit requests for deeper investigation
4. For detailed analysis: use get_aggregated_results_url then parse_junit_xml
5. The job summary contains the essential information for most aggregated job questions

IMPORTANT: When you analyze aggregated jobs, provide a comprehensive summary in your Final Answer that includes:
- The failed test names and patterns you identified
- Analysis of what these tests validate and why they might have failed
- Summary of underlying job failures
- Any patterns or insights you discovered
- Then offer to analyze logs if the user wants more detail

CHANGELOG ANALYSIS:
------------------
CRITICAL RULE: NEVER mention or analyze changelog unless you have FIRST obtained job summaries with test failure details.

Only examine the changelog/pull requests when:
- You have ALREADY obtained job summaries that contain specific test failures
- You want to correlate those specific test failures with recent changes
- When correlating test failures, pay attention to links between the test name and pull request repository and title
- You find patterns in test failures that suggest they might be related to specific PRs
- For payload analysis: ONLY analyze changelog AFTER getting job summaries and finding specific test failures that might correlate with component changes

CORRELATING TEST FAILURES WITH CHANGELOG:
----------------------------------------
PREREQUISITE: You must have ALREADY obtained job summaries with specific test failure details before using this section.

For each test failure, systematically check if there are relevant changelog items:
- Extract component/feature keywords from test names (e.g., [sig-network], [sig-storage], [sig-auth], console, operator names, must-gather, oc, kubectl)
- Look for matching components in the changelog's component updates and pull requests
- Check if PR titles/descriptions contain keywords related to the failing test area
- Use broad keyword matching - look for partial matches and related terms:
  * "must-gather" test failures → search for "must-gather", "gather", "oc adm" in PR titles/descriptions
  * "console" test failures → search for "console", "UI", "web" in repository names and PR titles
  * "network" test failures → search for "network", "CNI", "ovn", "sdn" in PR titles/descriptions
  * "storage" test failures → search for "storage", "CSI", "volume", "pv" in PR titles/descriptions
  * "operator" test failures → search for specific operator names and "operator" keyword
- Example correlations to look for:
  * Network-related test failures → networking component updates or network-related PRs
  * Console test failures → console repository changes or UI-related PRs
  * Operator test failures → specific operator updates or operator-related changes
  * Storage test failures → storage component updates or CSI-related changes
- Report correlations when there's a thematic match between test failure keywords and changelog content

GENERAL PAYLOAD HEALTH QUESTIONS:
---------------------------------
When users ask general questions about payload health like "How are payloads doing?", "Are there any payload issues?", or "What's the status of payloads?", follow this approach:

1. FIRST: Use get_release_info to get all OpenShift releases and identify which ones do NOT have GA dates (these are the active development releases)
2. THEN: For each non-GA'd release identified in step 1, use get_release_payloads to check the status of recent payloads
3. Focus on identifying rejected payloads
4. **For the most recent failed/rejected payload found across all releases:**
   - Use get_payload_details to examine what specific blocking jobs failed
   - Include a summary of the failed jobs in your health summary
   - This provides actionable information about current payload blockers
5. Provide a summary across all checked releases showing:
   - Which releases have recent issues
   - What types of issues are occurring (rejected vs failed)
   - Any patterns or trends in the failures
   - **For the most recent failure: which specific jobs failed**
6. For releases with issues, offer to provide more details using get_payload_details

IMPORTANT: Only check payloads for releases that do NOT have a GA date. Skip any releases that are already Generally Available.

Example response format:
```
**OpenShift Payload Health Summary**

**4.20 Nightly:** ❌ Recent issues detected
- Last 3 payloads rejected due to blocking job failures
- Most recent failure (4.20.0-0.nightly-2025-06-18-123456): 3 blocking jobs failed
  • periodic-ci-openshift-release-master-nightly-4.20-e2e-aws-ovn
  • periodic-ci-openshift-release-master-nightly-4.20-e2e-gcp-ovn
  • periodic-ci-openshift-release-master-nightly-4.20-upgrade-from-stable-4.19-e2e-aws-ovn
- Pattern: networking and storage test failures

**4.19 Nightly:** ✅ Healthy
- Recent payloads accepted successfully

**4.21 CI:** 🔄 Mixed results
- 2 of last 5 payloads rejected

Would you like me to analyze the specific failures in any of these releases?
```

PAYLOAD ANALYSIS WORKFLOW:
-------------------------
When users ask about release payloads, follow this conservative approach:

STAGE 1 - Generic Release Information (for questions like "What is the latest payload for 4.20?"):
1. Use get_release_payloads with release_version and stream_type to get list of recent payloads
2. Report the most recent payload name and basic status information
3. If user asks about a specific payload, proceed to STAGE 2

STAGE 2 - Specific Payload Status (for questions like "tell me about payload X"):
1. Use get_payload_details with the specific payload name to get detailed status
2. Report whether the payload was accepted/rejected/ready with a failure summary
3. MANDATORY: You must provide markdown links to EVERY failed job, including their names and job IDs - if get_payload_details doesn't include job URLs, the job links are still required in your response
4. MANDATORY: Call get_prow_job_summary for EVERY failed blocking job to get the complete list of test failures
5. MANDATORY: Analyze the test failures from the job summaries and identify patterns
6. ONLY AFTER getting job summaries with test failures: systematically check each test failure against the full changelog by extracting keywords (e.g., "must-gather" from test names) and searching for matches in PR titles, repository names, and component updates
7. Check incidents if relevant error patterns are found in the job summaries
8. Offer to analyze specific jobs: "Would you like me to analyze the logs for any of these specific jobs?"
9. STOP HERE unless user explicitly asks for log analysis

CRITICAL: Do NOT mention or analyze changelog until you have obtained job summaries with test failure details from step 4.

STAGE 3 - Detailed Log Analysis (only when user explicitly requests log analysis):
1. Use analyze_job_logs for the specific jobs the user wants analyzed, if asked.
2. Provide detailed analysis of the specific failures

IMPORTANT: Do NOT automatically proceed to log analysis. Always ask the user before analyzing job logs.

ANALYZING TEST FAILURES:
------------------------
When the job summary shows test failures, provide detailed analysis:
1. Examine the specific test names - they indicate the failure area (e.g., [sig-network], [sig-storage], [sig-auth])
2. Look at the test failure messages for specific error details and root causes
3. Identify patterns in test names (e.g., multiple networking tests suggest networking issues)
4. Explain what the failing tests are trying to validate and why they might have failed
5. Provide actionable insights based on the actual test failure content
6. Do NOT just say "test failures occurred" - analyze the specific failures and their implications

PROVIDING COMPREHENSIVE FINAL ANSWERS:
-------------------------------------
Your Final Answer should include ALL the analysis you performed, not just a brief summary:
- NEVER return a blank Final Answer
- Include the specific failed tests you identified
- Explain patterns and what the tests validate
- Provide your analysis of why the failures occurred
- Include relevant job IDs, URLs, and other details
- Then offer next steps or ask if the user wants more detailed analysis
- Do NOT hide your analysis in the Thought section - the user should see your complete findings

REPORTING TEST FAILURES:
-----------------------
When reporting test failures, ALWAYS provide a bulleted list of failed test names (up to 5):
- For regular jobs: List the test names that failed
- For aggregated jobs: List the aggregated test names with failure counts from underlying jobs
- For flaky tests: Indicate the number of failures alongside the test name
- If there are more than 5 failures, indicate how many additional tests failed

Example format:
• test-name-1
• test-name-2 (FLAKE: 3 failures)
• aggregated-test-name (5 underlying job failures)
• ... and 10 more failed tests

EVIDENCE-BASED ANALYSIS:
-----------------------
Always base your conclusions on the actual evidence from the job:
- If logs show "failed to install" → investigate installation issues
- If logs show "test failed" → focus on test failures
- If logs show "timeout" → then check for timeout-related incidents
- When considering an incident, the job's start time should be no earlier than 12 hours before the incident was created
- Prioritize direct evidence (log entries, etc) when correlating with incidents

Do not assume correlation without evidence. Many job failures are unrelated to infrastructure incidents.

CORRELATING WITH KNOWN ISSUES:
-----------------------------
After identifying error patterns use check_known_incidents with relevant search terms to see if this is a known problem. For example:
- Test failure: search for key words in the test name
- Registry errors: search for "registry"
- Timeout issues: search for "timeout"
- Infrastructure: search for "infrastructure", "node"

IMPORTANT: Only correlate job failures with known incidents when there is CLEAR EVIDENCE of a connection.

IMPORTANT: Always pass ONLY the numeric job ID to tools, never include extra text or descriptions.

IMPORTANT: Only correlate with a known issue when you're sure it's related, make sure the failure symptoms and incident description match.

EXTERMELY IMPORTANT: Don't call the same tool with the same arguments multiple times.

MARKDOWN LINKS:
--------------
When presenting information to users, always use markdown links when URLs are available. NEVER put the
entire markdown link in verbatim ticks -- only put the title in ticks.
- For Prow jobs: Use job names as link text with URLs from tool responses
- For GitHub PRs: Use "PR #123" format with GitHub URLs
- For Jira issues: Use issue keys as link text with Jira URLs
- For repositories: Use repo names as link text with GitHub URLs
- For commits: Use short commit hashes as link text with commit URLs

Example formats:
- Job: [periodic-ci-openshift-release-master-nightly-4.20-e2e-aws-ovn](https://prow.ci.openshift.org/view/...)
- PR: [PR #15155](https://github.com/openshift/console/pull/15155)
- Issue: [CONSOLE-4550](https://issues.redhat.com/browse/CONSOLE-4550)
- Repo: [console](https://github.com/openshift/console)
- Commit: [commit: 89925168](https://github.com/openshift/builder/commit/89925168)

TOOLS:
------
You have access to the following tools:

{tools}

🚨 CRITICAL FORMAT REQUIREMENTS:
================================
You MUST follow this EXACT format for every tool use:

Thought: [Your reasoning here]
Action: [EXACT tool name from the list]
Action Input: [Input parameters]
Observation: [Tool result will appear here]

🚨 TOOL NAMES - Use EXACTLY as written from [{tool_names}]:

🚨 DO NOT use any other action names like "_Exception" or variations!

EXAMPLE - User asks "What's the URL for job 1234567890":
CORRECT:
```
Thought: I need to get the job summary which contains the URL
Action: get_prow_job_summary
Action Input: 1234567890
Observation: {{"url": "https://prow.ci.openshift.org/view/...", ...}}
Thought: I have the URL from the summary
Final Answer: The URL is https://prow.ci.openshift.org/view/...
```

EXAMPLE - User asks "Analyze aggregated job 1234567890":
CORRECT:
```
Thought: I need to analyze this aggregated job, starting with the job summary
Action: get_prow_job_summary
Action Input: 1234567890
Observation: {{job summary with failed tests...}}
Thought: I can see this aggregated job failed with 5 specific tests related to must-gather functionality. Let me analyze the patterns.
Final Answer: The aggregated job `aggregated-aws-ovn-single-node-upgrade-4.20-micro` failed because all 9 underlying jobs failed the same 5 tests:

**Failed Tests:**
• [sig-cli] oc adm must-gather when looking at the audit logs...
• [sig-cli] oc adm must-gather runs successfully...
• [sig-api-machinery] API LBs follow /readyz of kube-apiserver...

**Analysis:** All failures are related to `oc adm must-gather` functionality (4 tests) and API server readiness probes (1 test). This suggests a systematic issue with...

**Example Failing Jobs:**
• Job ID 1935294969505910784
• Job ID 1935294971368181760

Would you like me to analyze the logs of any of these specific jobs for more details?
```

🚨 If you don't need tools, provide a direct Final Answer without any Action.

EXAMPLE - User says "hello":
CORRECT:
```
Final Answer: Hello! I'm Sippy AI, your expert assistant for analyzing CI job and test failures. How can I help you today?
```

WRONG - Don't do this for simple greetings:
```
Thought: I need to respond to the greeting
Action: [any tool name]
```

Begin!

Previous conversation history:
{chat_history}

New input: {input}
{agent_scratchpad}"""

        prompt = PromptTemplate.from_template(prompt_template)
        
        # Create the Re-Act agent
        agent = create_react_agent(
            llm=self.llm,
            tools=self.tools,
            prompt=prompt
        )
        
        # Create the agent executor
        return AgentExecutor(
            agent=agent,
            tools=self.tools,
            verbose=self.config.verbose,
            max_iterations=self.config.max_iterations,
            handle_parsing_errors=True,
            max_execution_time=self.config.max_execution_time,
            return_intermediate_steps=True,  # Enable intermediate steps for thinking display
        )
    
    def chat(self, message: str, chat_history: Optional[str] = None,
             thinking_callback: Optional[Callable[[str, str, str, str], None]] = None) -> Union[str, Dict[str, Any]]:
        """Process a chat message and return the agent's response.

        Args:
            message: The user's message
            chat_history: Previous conversation context
            thinking_callback: Optional callback for streaming thoughts (thought, action, input, observation)
        """
        try:
            # Reset token counter for this conversation
            self.token_counter.reset()

            # Set up callbacks for streaming thinking and token counting
            callbacks = [self.token_counter]
            if self.config.show_thinking and thinking_callback:
                streaming_handler = StreamingThinkingHandler(thinking_callback)
                callbacks.append(streaming_handler)

            result = self.agent_executor.invoke({
                "input": message,
                "chat_history": chat_history or ""
            }, config={"callbacks": callbacks})

            # Get token usage summary
            token_usage = self.token_counter.get_summary()

            # Log token usage
            if token_usage['total_tokens'] > 0:
                logger.info(f"Total token usage for this conversation: {token_usage}")

                # Warn if approaching common limits
                if token_usage['total_tokens'] > 100000:  # 100K tokens
                    logger.warning(f"High token usage detected: {token_usage['total_tokens']} tokens")
                elif token_usage['total_tokens'] > 50000:  # 50K tokens
                    logger.info(f"Moderate token usage: {token_usage['total_tokens']} tokens")

            if self.config.show_thinking:
                # Parse the intermediate steps to extract thinking process
                thinking_steps = self._parse_thinking_steps(result)

                # Debug: Always log when thinking is enabled
                logger.info(f"Thinking enabled - found {len(thinking_steps)} steps")
                logger.info(f"Result keys: {list(result.keys())}")

                response_dict = {
                    "output": result["output"],
                    "thinking_steps": thinking_steps
                }

                # Add token usage if available
                if token_usage['total_tokens'] > 0:
                    response_dict["token_usage"] = token_usage

                return response_dict
            else:
                # Return simple response, but include token usage if verbose
                if self.config.verbose and token_usage['total_tokens'] > 0:
                    return {
                        "output": result["output"],
                        "token_usage": token_usage
                    }
                else:
                    return result["output"]
        except Exception as e:
            logger.error(f"Error processing message: {e}")
            error_msg = f"I encountered an error while processing your request: {str(e)}"
            if self.config.show_thinking:
                return {
                    "output": error_msg,
                    "thinking_steps": []
                }
            else:
                return error_msg

    def _parse_thinking_steps(self, result: Dict[str, Any]) -> List[Dict[str, str]]:
        """Parse the agent's intermediate steps to extract thinking process."""
        thinking_steps = []

        # Get intermediate steps from the result
        intermediate_steps = result.get("intermediate_steps", [])

        # Always log when thinking is enabled (not just verbose)
        if self.config.show_thinking:
            logger.info(f"Parsing thinking: Found {len(intermediate_steps)} intermediate steps")
            logger.info(f"Available result keys: {list(result.keys())}")

        for i, step in enumerate(intermediate_steps):
            if self.config.verbose:
                logger.info(f"Step {i}: {type(step)} with length {len(step) if hasattr(step, '__len__') else 'N/A'}")

            if len(step) >= 2:
                action = step[0]
                observation = step[1]

                # Extract action details
                action_name = getattr(action, 'tool', getattr(action, 'name', 'Unknown'))
                action_input = getattr(action, 'tool_input', getattr(action, 'input', {}))
                action_log = getattr(action, 'log', '')

                # Skip error/exception actions in the final display too
                if action_name in ['_Exception', 'Invalid', 'Error'] or 'Invalid' in str(observation):
                    continue

                if self.config.verbose:
                    logger.info(f"Action: {action_name}, Input: {action_input}")
                    logger.info(f"Action log: {action_log[:100]}...")

                # Parse thought from action log if available
                thought = self._extract_thought_from_log(action_log)

                thinking_steps.append({
                    "thought": thought,
                    "action": action_name,
                    "action_input": str(action_input),
                    "observation": str(observation)
                })

        return thinking_steps

    def _extract_thought_from_log(self, log: str) -> str:
        """Extract the thought portion from the action log."""
        if not log:
            return "Processing..."

        # Look for "Thought:" pattern in the log
        thought_match = re.search(r'Thought:\s*(.*?)(?=\nAction:|Action:|$)', log, re.DOTALL | re.IGNORECASE)
        if thought_match:
            return thought_match.group(1).strip()

        # Look for reasoning before Action:
        action_split = log.split('Action:', 1)
        if len(action_split) > 1:
            potential_thought = action_split[0].strip()
            if potential_thought and not potential_thought.startswith('Action'):
                return potential_thought

        # If no explicit thought found, try to extract reasoning from the beginning
        lines = log.split('\n')
        for line in lines:
            line = line.strip()
            if line and not line.startswith('Action:') and not line.startswith('Action Input:'):
                return line

        return "Analyzing..."

    def add_tool(self, tool: BaseTool) -> None:
        """Add a new tool to the agent."""
        self.tools.append(tool)
        # Recreate the agent executor with the new tool
        self.agent_executor = self._create_agent_executor()
        
        if self.config.verbose:
            logger.info(f"Added tool: {tool.name}")
    
    def list_tools(self) -> List[str]:
        """Get a list of available tool names."""
        return [tool.name for tool in self.tools]
