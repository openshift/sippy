"""
Core Re-Act agent implementation for Sippy using LangGraph.
"""

import logging
import asyncio
from typing import List, Optional, Union, Dict, Any, Callable, Awaitable
from langchain_core.messages import HumanMessage, AIMessage, BaseMessage
from langchain_openai import ChatOpenAI
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain.tools import BaseTool

from .config import Config
from .api_models import ChatMessage
from .graph import create_react_graph, extract_thinking_steps, get_final_response
from .tools import (
    SippyProwJobSummaryTool,
    SippyLogAnalyzerTool,
    SippyJiraIncidentTool,
    SippyJiraIssueTool,
    SippyReleasePayloadTool,
    SippyPayloadDetailsTool,
    JUnitParserTool,
    AggregatedJobAnalyzerTool,
    AggregatedYAMLParserTool,
    SippyDatabaseQueryTool,
    SippyTestDetailsTool,
    load_tools_from_mcp,
)

logger = logging.getLogger(__name__)

class SippyAgent:
    """LangGraph Re-Act agent for CI analysis with Sippy."""

    def __init__(self, config: Config):
        """Initialize the Sippy agent with configuration."""
        self.config = config
        self.llm = self._create_llm()
        self.tools = asyncio.run(self._create_tools())
        self.graph = self._create_agent_graph()

    def _create_llm(self) -> Union[ChatOpenAI, ChatGoogleGenerativeAI]:
        """Create the language model instance."""
        if self.config.verbose:
            logger.info(f"Creating LLM with endpoint: {self.config.llm_endpoint}")
            logger.info(f"Using model: {self.config.model_name}")

        # Use ChatGoogleGenerativeAI for Gemini models
        if self.config.is_gemini_model():
            if (
                not self.config.google_api_key
                and not self.config.google_credentials_file
            ):
                raise ValueError(
                    "Google API key or service account credentials file is required for Gemini models"
                )

            llm_kwargs = {
                "model": self.config.model_name,
                "temperature": self.config.temperature,
                "include_thoughts": self.config.show_thinking,
            }

            # Use API key if provided, otherwise use service account credentials
            if self.config.google_api_key:
                llm_kwargs["google_api_key"] = self.config.google_api_key
                if self.config.verbose:
                    logger.info(
                        f"Using ChatGoogleGenerativeAI for Gemini model with API key"
                    )
            elif self.config.google_credentials_file:
                # Set the environment variable for Google credentials
                import os

                os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = (
                    self.config.google_credentials_file
                )
                if self.config.verbose:
                    logger.info(
                        f"Using ChatGoogleGenerativeAI for Gemini model with service account: {self.config.google_credentials_file}"
                    )

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
                logger.info(
                    f"Using ChatOpenAI with base_url: {self.config.llm_endpoint}"
                )

            return ChatOpenAI(**llm_kwargs)

    async def _create_tools(self) -> List[BaseTool]:
        """Create the list of tools available to the agent."""
        tools = [
            SippyProwJobSummaryTool(sippy_api_url=self.config.sippy_api_url),
            SippyLogAnalyzerTool(sippy_api_url=self.config.sippy_api_url),
            SippyTestDetailsTool(sippy_api_url=self.config.sippy_api_url),
            SippyJiraIncidentTool(
                jira_url=self.config.jira_url,
                jira_username=self.config.jira_username,
                jira_token=self.config.jira_token,
            ),
            SippyJiraIssueTool(
                jira_url=self.config.jira_url,
            ),
            SippyReleasePayloadTool(),
            SippyPayloadDetailsTool(),
            JUnitParserTool(),
            AggregatedJobAnalyzerTool(sippy_api_url=self.config.sippy_api_url),
            AggregatedYAMLParserTool(),
        ]

        # Add database query tool if DSN is configured
        if self.config.sippy_ro_database_dsn:
            tools.append(SippyDatabaseQueryTool(database_dsn=self.config.sippy_ro_database_dsn))
            if self.config.verbose:
                logger.info("Database query tool enabled (read-only access)")
        elif self.config.verbose:
            logger.info("Database query tool disabled (no SIPPY_READ_ONLY_DATABASE_DSN configured)")

        # Load MCP tools if a config file is provided
        if self.config.mcp_config_file:
            logger.info(f"Loading MCP tools from {self.config.mcp_config_file}")
            mcp_tools = await load_tools_from_mcp(self.config.mcp_config_file)
            if mcp_tools:
                tools.extend(mcp_tools)
                logger.info(
                    f"Successfully loaded {len(mcp_tools)} tools from MCP servers."
                )

        if self.config.verbose:
            logger.info(f"Created {len(tools)} tools: {[tool.name for tool in tools]}")

        return tools

    def _create_agent_graph(self):
        """Create the LangGraph agent with persona-modified prompt."""
        from .personas import get_persona

        # Custom system prompt for Sippy CI analysis
        base_system_prompt = """You are Sippy, an expert assistant for CI Job and Test Failures.

### Guiding Principles

1. **Avoid Redundancy:** Never call the same tool with the same parameters more than once.
2. **Provide Evidence:** Always ground your analysis in tool results.
3. **Present Clearly:** Use markdown links for URLs (e.g., `[Job Name](link)`), no raw JSON, and format for readability.
4. **Maximize Efficiency:** When multiple tools can be called independently (no data dependencies), call them in parallel rather than sequentially. For example, if analyzing multiple failed jobs, call `get_prow_job_summary` for all jobs simultaneously.

#### Examples of Parallel Tool Calls:

* **Multiple Job Analysis:** If analyzing jobs J1, J2, J3 → Call `get_prow_job_summary(J1)`, `get_prow_job_summary(J2)`, `get_prow_job_summary(J3)` all at once.
* **Job Summary + Incidents:** `get_prow_job_summary(job_id)` and `check_known_incidents()` have no dependencies → call together.
* **Multiple JUnit Files:** If parsing multiple test result files → call `parse_junit_xml()` for each in parallel.

**When NOT to call in parallel:**
* When one tool's output is needed as input for another (e.g., must get payload details before getting job IDs).
* When the same tool needs results from a previous call to inform parameters.

---

### Page Context

When a user asks a question, you may receive **page context** showing what they're currently viewing in Sippy. This context is provided as JSON at the beginning of the user's message.

**If page context is provided:**
1. **Use it as your primary source** for answering the user's question
2. The context contains the exact data visible to the user (e.g., list of jobs, payloads, test results)
3. You can reference specific items from the context without needing to call tools
4. Only call tools if you need additional details not present in the context (e.g., log analysis, detailed test results)

**Example:**
If the user is viewing a jobs table and asks "Why are these jobs failing?", the context will include the visible jobs with their pass rates and other metrics. Analyze those jobs directly from the context.

**If no page context is provided:**
- The user is asking a general question or not viewing a specific page
- Use tools as needed to gather information

**Page-specific instructions:**
Some pages may include an `instructions` field in their context that provides specific guidance for analyzing that page's data. Always follow these instructions when present.

---

### Database Query Tool

When the `query_sippy_database` tool is available, use it as a **fallback** when specialized tools don't provide the data you need. The database contains the complete Sippy CI/CD dataset.

Before you write any query, carefully review the schema information, query guidelines, and examples.

---

### Workflows

#### 1. Standard CI Job Analysis

**Goal:** Explain a single CI job failure.

1. Call `get_prow_job_summary` with the job ID.
2. If that’s enough to answer the question → stop.
3. If not, ask the user if you should analyze logs with `analyze_job_logs`.

#### 2. Aggregated Job Analysis

**Goal:** Analyze `aggregated-*` jobs.

1. Start with `get_prow_job_summary` and report the failed tests.
2. Only go deeper if the user asks about “underlying jobs.”
3. For deeper analysis: `get_aggregated_results_url` → `parse_junit_xml`.

#### 3. General Payload Health Analysis

**Goal:** Broad questions like *“How are the release payloads doing?”*

1. Get active releases via `get_release_info`.
2. Use `get_release_payloads` for recent payload statuses, do not include ready payloads unless the user asks for them.
3. Filter the retrieved payloads to exclude any in the 'Ready' state.  Then from the remaining payloads, identify the most recent one.
4. Call `get_payload_details` for blocking jobs, if the payload is rejected.
5. Summarize payload health and highlight what blocked the payload, if anything. 

#### 4. Specific Payload Investigation

**Goal:** Explain why a payload (e.g., X) was rejected.

1. Use `get_payload_details` to list failed blocking jobs.
2. For **all failed blocking jobs**, call `get_prow_job_summary` **in parallel** to get failed tests (these are independent calls).
3. **Always check `check_known_incidents`** to see if failures correlate with ongoing issues.

4. Synthesize results:
   * Report failed jobs + tests.
   * Highlight patterns or correlations with incidents.
5. If no incident matches, analyze the payload changelog for possible causes.
6. Offer optional detailed log analysis with `analyze_job_logs`.

---

### Analysis & Reporting Rules

#### Reporting Test Failures

* List up to 5 failing tests explicitly. Summarize extras (e.g., “…and 3 more failed”).
* Explain what those tests validate and why they might fail.

#### Correlating Failures with Changes

* Do **not** analyze changelog until after identifying test failures.
* Match failure keywords (e.g., *networking, storage*) to PR components or repos.
* Only report correlations when there’s a clear thematic link.

#### Correlating Failures with Incidents

* Always use `check_known_incidents` when analyzing payload failures.
* Prefer log evidence, but note correlations if timing and symptoms align.

#### Final Answer Composition

Your final answer must be **comprehensive**:

* List failing jobs and tests.
* Explain likely causes.
* Include relevant links (Jobs, PRs, Issues, Incidents).
* Suggest the next logical step (e.g., *“Would you like me to analyze the logs?”*).
"""

        # Apply persona modification (always prepend if present)
        persona = get_persona(self.config.persona)

        if persona.system_prompt_modifier:
            system_prompt = persona.system_prompt_modifier + base_system_prompt
        else:
            system_prompt = base_system_prompt

        # Create the LangGraph react agent
        return create_react_graph(
            llm=self.llm,
            tools=self.tools,
            system_prompt=system_prompt,
            max_iterations=self.config.max_iterations,
        )

    async def achat(
        self,
        message: str,
        chat_history: Optional[List[ChatMessage]] = None,
        thinking_callback: Optional[
            Callable[[str, str, str, str], Awaitable[None]]
        ] = None,
    ) -> Union[str, Dict[str, Any]]:
        """Process a chat message and return the agent's response.

        Args:
            message: The user's message
            chat_history: Previous conversation context as a list of ChatMessage objects
            thinking_callback: Optional async callback for streaming thoughts (thought, action, input, observation)
        """
        try:
            # Build message history
            history_messages: List[BaseMessage] = []
            if chat_history:
                for msg in chat_history:
                    if msg.role == "user":
                        history_messages.append(HumanMessage(content=msg.content))
                    elif msg.role == "assistant":
                        history_messages.append(AIMessage(content=msg.content))

            # Add the current user message
            history_messages.append(HumanMessage(content=message))

            return await self._achat_streaming(history_messages, thinking_callback)

        except Exception as e:
            logger.error(f"Error processing message: {e}", exc_info=True)
            error_msg = (
                f"I encountered an error while processing your request: {str(e)}"
            )
            if self.config.show_thinking:
                return {"output": error_msg, "thinking_steps": []}
            else:
                return error_msg

    async def _achat_streaming(
        self,
        history_messages: List[BaseMessage],
        thinking_callback: Optional[Callable[[str, str, str, str], Awaitable[None]]] = None,
    ) -> Union[str, Dict[str, Any]]:
        """Process messages and optionally stream the agent's thinking process."""
        all_messages = []
        thinking_steps = []
        current_tool_calls = {}  # Track tool calls by tool_call_id
        thought_buffer = []  # Buffer for accumulating thoughts

        # Stream events from the graph
        async for event in self.graph.astream_events(
            {"messages": history_messages, "iterations": 0},
            version="v2",
        ):
            kind = event.get("event")
            data = event.get("data", {})

            # Capture streaming chunks to extract thoughts
            if kind == "on_chat_model_stream":
                chunk = data.get("chunk")
                
                if chunk and hasattr(chunk, "content"):
                    content = chunk.content
                    
                    # Handle Gemini's structured content with thoughts
                    if isinstance(content, list):
                        for part in content:
                            if isinstance(part, dict):
                                # Check for thinking content (Gemini uses 'type': 'thinking')
                                if part.get("type") == "thinking" and "thinking" in part:
                                    thought_text = part.get("thinking", "")
                                    if thought_text:
                                        if self.config.verbose:
                                            logger.debug(f"Captured thought: {thought_text[:100]}...")
                                        thought_buffer.append(thought_text)
                                        # Stream the thought if callback provided
                                        if thinking_callback and self.config.show_thinking:
                                            await thinking_callback(
                                                thought_text,
                                                "thinking",
                                                "",
                                                "",
                                            )

            # When agent makes a tool call
            if kind == "on_chat_model_end":
                output = data.get("output")
                if hasattr(output, "tool_calls") and output.tool_calls:
                    for tool_call in output.tool_calls:
                        tool_call_id = tool_call.get("id")
                        tool_name = tool_call.get("name", "Unknown")
                        tool_input = tool_call.get("args", {})

                        # Store tool call for later matching with results
                        current_tool_calls[tool_call_id] = {
                            "name": tool_name,
                            "input": tool_input,
                            "thought": f"Using tool: {tool_name}",
                        }

                        # Stream the tool call start immediately if callback provided
                        if thinking_callback:
                            await thinking_callback(
                                current_tool_calls[tool_call_id]["thought"],
                                tool_name,
                                str(tool_input),
                                "",  # No observation yet
                            )

            # When a tool returns results
            elif kind == "on_tool_end":
                output = data.get("output")
                # Get the input data which contains the tool_call_id
                input_data = data.get("input", {})

                # Try to find the tool call ID from the input
                tool_call_id = None
                if isinstance(input_data, dict):
                    tool_call_id = input_data.get("tool_call_id")

                # If we can't find by ID, try to match by name
                tool_name = event.get("name", "")
                matched_call = None

                if tool_call_id and tool_call_id in current_tool_calls:
                    matched_call = current_tool_calls[tool_call_id]
                    del current_tool_calls[tool_call_id]
                else:
                    # Fallback: match by name (first unmatched call with this name)
                    for call_id, call_data in list(current_tool_calls.items()):
                        if call_data["name"] == tool_name:
                            matched_call = call_data
                            del current_tool_calls[call_id]
                            break

                if matched_call:
                    observation = str(output) if output else ""

                    # Stream the observation if callback provided
                    if thinking_callback:
                        await thinking_callback(
                            matched_call["thought"],
                            matched_call["name"],
                            str(matched_call["input"]),
                            observation,
                        )

                    # Add to thinking steps
                    thinking_steps.append(
                        {
                            "thought": matched_call["thought"],
                            "action": matched_call["name"],
                            "action_input": str(matched_call["input"]),
                            "observation": observation,
                        }
                    )

            # Collect all messages for final response
            if kind in ["on_chat_model_stream", "on_chat_model_end"]:
                output = data.get("output")
                if output and isinstance(output, AIMessage):
                    all_messages.append(output)
        
        # Extract final response
        final_response = get_final_response(all_messages)

        if self.config.show_thinking:
            # Add accumulated thoughts at the beginning if any
            if thought_buffer:
                thinking_steps.insert(0, {
                    "thought": "\n\n".join(thought_buffer),
                    "action": "thinking",
                    "action_input": "",
                    "observation": "",
                })
            
            if thinking_steps:
                return {
                    "output": final_response,
                    "thinking_steps": thinking_steps,
                }
        
        return final_response

    def add_tool(self, tool: BaseTool) -> None:
        """Add a new tool to the agent."""
        self.tools.append(tool)
        # Recreate the graph with the new tool
        self.graph = self._create_agent_graph()

        if self.config.verbose:
            logger.info(f"Added tool: {tool.name}")

    def list_tools(self) -> List[str]:
        """Get a list of available tool names."""
        return [tool.name for tool in self.tools]
