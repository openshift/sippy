"""
Core Re-Act agent implementation for Sippy using LangGraph.
"""

import logging
import asyncio
from typing import List, Optional, Union, Dict, Any, Callable, Awaitable
from langchain_core.messages import HumanMessage, AIMessage, BaseMessage
from langchain_openai import ChatOpenAI
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_google_vertexai.model_garden import ChatAnthropicVertex
from langchain.tools import BaseTool

from .config import Config, ModelConfig, load_models_config
from .api_models import ChatMessage
from .graph import create_react_graph, extract_thinking_steps, get_final_response
from .tools import (
    SippyProwJobSummaryTool,
    SippyProwJobPayloadTool,
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
    TriagePotentialMatchesTool,
    load_tools_from_mcp,
)
from . import metrics

logger = logging.getLogger(__name__)

class SippyAgent:
    """LangGraph Re-Act agent for CI analysis with Sippy."""

    def __init__(self, config: Config):
        """Initialize the Sippy agent with configuration."""
        self.config = config
        self.llm = self._create_llm()
        self.tools = None  # Will be initialized asynchronously
        self.graph = None  # Will be initialized after tools are loaded
        self._initialized = False
    
    async def _initialize(self):
        """Asynchronously initialize tools and graph."""
        if not self._initialized:
            self.tools = await self._create_tools()
            self.graph = self._create_agent_graph()
            self._initialized = True

    def _create_llm(self) -> Union[ChatOpenAI, ChatGoogleGenerativeAI, ChatAnthropicVertex]:
        """Create the language model instance."""
        if self.config.verbose:
            logger.info(f"Creating LLM with endpoint: {self.config.llm_endpoint}")
            logger.info(f"Using model: {self.config.model_name}")

        # Use ChatAnthropicVertex for Claude models via Vertex AI
        if self.config.is_claude_model():
            if not self.config.google_project_id:
                raise ValueError(
                    "Google Cloud project ID is required for Claude models via Vertex AI"
                )

            # Set credentials file if provided, otherwise use Application Default Credentials (gcloud auth)
            if self.config.google_credentials_file:
                import os
                os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = self.config.google_credentials_file
                if self.config.verbose:
                    logger.info(f"Using explicit credentials: {self.config.google_credentials_file}")
            else:
                if self.config.verbose:
                    logger.info("Using Application Default Credentials (gcloud auth)")

            # Enable extended thinking for Claude if show_thinking is enabled
            # Note: Extended thinking requires temperature=1 and max_tokens > budget_tokens
            enable_extended_thinking = self.config.show_thinking and self.config.extended_thinking_budget > 0
            
            llm_kwargs = {
                "model_name": self.config.model_name,
                "project": self.config.google_project_id,
                "location": self.config.google_location,
                "temperature": 1.0 if enable_extended_thinking else self.config.temperature,
            }

            if enable_extended_thinking:
                # max_tokens must be greater than thinking budget
                # Claude's max output is 64K tokens
                max_tokens = 64000
                
                llm_kwargs["max_tokens"] = max_tokens
                llm_kwargs["model_kwargs"] = {
                    "thinking": {
                        "type": "enabled",
                        "budget_tokens": self.config.extended_thinking_budget
                    }
                }
                if self.config.verbose:
                    logger.info(f"Extended thinking enabled with budget: {self.config.extended_thinking_budget} tokens")
                    logger.info(f"Max tokens set to {max_tokens}")
                    logger.info("Temperature automatically set to 1.0 (required for extended thinking)")
            elif self.config.show_thinking:
                if self.config.verbose:
                    logger.info("Extended thinking disabled (budget=0)")

            if self.config.verbose:
                logger.info(
                    f"Using ChatAnthropicVertex for Claude model: {self.config.model_name} "
                    f"(project: {self.config.google_project_id}, location: {self.config.google_location})"
                )

            return ChatAnthropicVertex(**llm_kwargs)

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
            SippyProwJobPayloadTool(sippy_api_url=self.config.sippy_api_url),
            SippyLogAnalyzerTool(sippy_api_url=self.config.sippy_api_url),
            SippyTestDetailsTool(sippy_api_url=self.config.sippy_api_url),
            SippyJiraIncidentTool(
                jira_url=self.config.jira_url,
                jira_username=self.config.jira_username,
                jira_token=self.config.jira_token,
            ),
            SippyJiraIssueTool(jira_url=self.config.jira_url),
            TriagePotentialMatchesTool(sippy_api_url=self.config.sippy_api_url),
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
        else:
            logger.warning("Database query tool disabled: SIPPY_READ_ONLY_DATABASE_DSN not configured")

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

    def _create_agent_graph(self, persona: Optional[str] = None):
        """Create the LangGraph agent with persona-modified prompt.

        Args:
            persona: Optional persona name to use. If None, uses self.config.persona.
        """
        from .personas import get_persona

        # Use provided persona or fall back to config
        persona_name = persona if persona is not None else self.config.persona

        # Custom system prompt for Sippy CI analysis
        base_system_prompt = """You are Sippy, an expert assistant for CI Job and Test Failures.  You carefully consider the user's question and use your available tools and knowledge to answer the question.

### Guiding Principles

1. **Use your available tools:** Always use your available tools to answer the user's question.
2. **Avoid Redundancy:** Never call the same tool with the same parameters more than once.
3. **Provide Evidence:** Always ground your analysis in tool results.
4. **Present Clearly:** Use markdown links for URLs (e.g., `[Job Name](link)`), no raw JSON, and format for readability. When constructing markdown links, if the link text contains its own brackets ([ or ]), escape them with a backslash to ensure it is rendered correctly.
5. **Maximize Efficiency:** When multiple tools can be called independently (no data dependencies), call them in parallel rather than sequentially. For example, if analyzing multiple failed jobs, call `get_prow_job_summary` for all jobs simultaneously.
6. When a tool argument (especially a URL) is explicitly described as requiring its value "verbatim," "exactly as provided," or "without modification," you MUST pass the provided string directly to the tool without any internal parsing, re-construction, or alteration of its content. Treat such arguments as opaque strings.

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
5. If you have previously called a tool, re-use the information unless you need to call it again with different inputs. For example,
if you previously called check_known_incidents you can re-use that information without calling it again.

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

**Goal:** Broad questions like *“How are the release payloads doing?”* or "How are the release payloads doing for 4.21?"

1. If the user didn't specify a release, get releases via `get_release_info`. Use the very first one in the list.
2. Use `get_release_payloads` for recent payload statuses, do not include ready payloads unless the user asks for them.
3. Filter the retrieved payloads to exclude any in the 'Ready' state.  Then from the remaining payloads, identify the most recent one.
4. You must analyze the most recent payload if it is rejected.
5. Call `get_payload_details` for blocking jobs on the payload if it is rejected.
6. Analyze the blocking jobs, summarize the results and highlight the root cause.  Use the check_known_incidents tool to see if the failures are correlated with ongoing incidents.
7. DO NOT REPORT ON READY PAYLOADS UNLESS ASKED.

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

#### 5. Incidents

Incidents are tracked in Jira. If the user asks, call the `check_known_incidents` tool to see if there's any open incidents.

---

### Analysis & Reporting Rules

#### Reporting Test Failures

* List up to 5 failing tests explicitly. Summarize extras (e.g., "…and 3 more failed").
* Explain what those tests validate and why they might fail.

#### Correlating Failures with Changes

* Do **not** analyze changelog until after identifying test failures.
* Match failure keywords (e.g., *networking, storage*) to PR components or repos.
* Only report correlations when there's a clear thematic link.

#### Correlating Failures with Incidents

* Always use `check_known_incidents` when analyzing payload failures.
* Prefer log evidence, but note correlations if timing and symptoms align.

#### Creating Visualizations

When users request visual representations (e.g., "plot", "graph", "chart", "visualize"), you can create interactive Plotly charts directly in your response.

**How to create a visualization:**

1. After your main text response, include a visualization block using these exact markers:
   ```
   VISUALIZATION_START
   {{
     "data": [...],
     "layout": {{...}},
     "config": {{...}}
   }}
   VISUALIZATION_END
   ```

2. The JSON must be valid Plotly specification with three fields:
   - **data**: Array of trace objects (required)
   - **layout**: Layout configuration object (required)
   - **config**: Optional config object for controls

**Example - Line chart for test success rates over time:**
```
Here's the trend for the test over the last 7 days:

VISUALIZATION_START
{{
  "data": [
    {{
      "x": ["2025-10-08", "2025-10-09", "2025-10-10", "2025-10-11", "2025-10-12", "2025-10-13", "2025-10-14"],
      "y": [85, 82, 90, 88, 91, 89, 92],
      "type": "scatter",
      "mode": "lines+markers",
      "name": "Success Rate",
      "line": {{"color": "#4caf50", "width": 3}},
      "marker": {{"size": 8}}
    }}
  ],
  "layout": {{
    "title": {{"text": "Test Success Rate - Last 7 Days"}},
    "xaxis": {{"title": "Date"}},
    "yaxis": {{"title": "Success Rate (%)", "range": [0, 100]}},
    "hovermode": "x unified"
  }}
}}
VISUALIZATION_END
```

**Common chart types:**
- **Line charts**: `"type": "scatter", "mode": "lines+markers"` - for trends over time
- **Bar charts**: `"type": "bar"` - for comparisons across categories
- **Scatter plots**: `"type": "scatter", "mode": "markers"` - for correlations
- **Multi-series**: Include multiple objects in the `data` array

**Important:**
- Only create visualizations when the user explicitly requests them or when visual data would significantly enhance understanding
- Always provide text analysis alongside the visualization
- Use colors that work in both light and dark modes
- Keep it simple - don't include excessive styling

**Color Guidelines:**
- **Success/passing data**: Use green shades
- **Failure/error data**: Use red shade
- **Multiple categories**: When showing multiple distinct categories (not success/failure), use colors that make sense for the data
- Ensure colors have sufficient contrast for readability in both light and dark themes

#### Final Answer Composition

Your final answer must be **comprehensive**:

* List failing jobs and tests.
* Explain likely causes.
* Include relevant links (Jobs, PRs, Issues, Incidents).
* Include visualizations when requested or when they add significant value.
* Suggest the next logical step (e.g., *"Would you like me to analyze the logs?"*).
"""

        # Apply persona modification (always prepend if present)
        persona_obj = get_persona(persona_name)

        if persona_obj.system_prompt_modifier:
            system_prompt = persona_obj.system_prompt_modifier + base_system_prompt
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
        persona: Optional[str] = None,
        show_thinking: Optional[bool] = None,
    ) -> Union[str, Dict[str, Any]]:
        """Process a chat message and return the agent's response.

        Args:
            message: The user's message
            chat_history: Previous conversation context as a list of ChatMessage objects
            thinking_callback: Optional async callback for streaming thoughts (thought, action, input, observation)
            persona: Optional persona override for this request. If None, uses self.config.persona
            show_thinking: Optional show_thinking override for this request. If None, uses self.config.show_thinking
        """
        # Ensure agent is fully initialized
        await self._initialize()

        # Determine effective persona and show_thinking for this request
        effective_persona = persona if persona is not None else self.config.persona
        effective_show_thinking = show_thinking if show_thinking is not None else self.config.show_thinking

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

            return await self._achat_streaming(
                history_messages,
                thinking_callback,
                effective_persona,
                effective_show_thinking
            )

        except Exception as e:
            logger.error(f"Error processing message: {e}", exc_info=True)
            error_msg = (
                f"I encountered an error while processing your request: {str(e)}"
            )
            if effective_show_thinking:
                return {"output": error_msg, "thinking_steps": []}
            else:
                return error_msg

    async def _achat_streaming(
        self,
        history_messages: List[BaseMessage],
        thinking_callback: Optional[Callable[[str, str, str, str], Awaitable[None]]] = None,
        persona: Optional[str] = None,
        show_thinking: Optional[bool] = None,
    ) -> Union[str, Dict[str, Any]]:
        """Process messages and optionally stream the agent's thinking process.

        Args:
            history_messages: Message history
            thinking_callback: Optional callback for streaming thinking steps
            persona: Optional persona override for this request
            show_thinking: Optional show_thinking override for this request
        """
        all_messages = []
        thinking_steps = []
        current_tool_calls = {}  # Track tool calls by tool_call_id
        thought_buffer = []  # Buffer for accumulating complete thoughts
        current_thinking_chunk = []  # Buffer for accumulating Claude's token-by-token thinking

        # Determine effective persona and show_thinking
        effective_persona = persona if persona is not None else self.config.persona
        effective_show_thinking = show_thinking if show_thinking is not None else self.config.show_thinking

        # Create a graph with the specified persona (avoid mutating self.graph)
        request_graph = self._create_agent_graph(effective_persona)

        # Determine if this is a Gemini model (sends complete thoughts) vs Claude (streams tokens)
        is_gemini = self.config.is_gemini_model()

        # Stream events from the graph
        async for event in request_graph.astream_events(
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
                    
                    # Handle structured content with thoughts (Gemini and Claude)
                    if isinstance(content, list):
                        for part in content:
                            if isinstance(part, dict):
                                # Check for thinking content
                                if part.get("type") == "thinking" and "thinking" in part:
                                    thought_text = part.get("thinking", "")
                                    if thought_text:
                                        if is_gemini:
                                            # Gemini sends complete thoughts each turn - stream immediately
                                            if self.config.verbose:
                                                logger.debug(f"Gemini complete thought: {thought_text[:50]}...")
                                            
                                            thought_buffer.append(thought_text)

                                            # Stream the complete thought immediately if callback provided
                                            if thinking_callback and effective_show_thinking:
                                                await thinking_callback(
                                                    thought_text,
                                                    "thinking",
                                                    "",
                                                    "",
                                                )
                                        else:
                                            # Claude streams thinking token-by-token, accumulate it
                                            current_thinking_chunk.append(thought_text)
                                            
                                            if self.config.verbose:
                                                logger.debug(f"Claude thinking token: {thought_text[:50]}...")
                    
                    # Also check for thinking content blocks (alternative format)
                    elif isinstance(content, str) and content:
                        # Some models might send thinking as regular text chunks
                        # We'll handle this in on_chat_model_end
                        pass
            
            # When model completes a response, process accumulated thinking (Claude only)
            elif kind == "on_chat_model_end":
                # If we accumulated thinking chunks (Claude), combine and stream them
                if current_thinking_chunk:
                    complete_thought = "".join(current_thinking_chunk)
                    thought_buffer.append(complete_thought)
                    
                    if self.config.verbose:
                        logger.debug(f"Complete Claude thought accumulated ({len(complete_thought)} chars)")

                    # Stream the complete thought if callback provided
                    if thinking_callback and effective_show_thinking:
                        await thinking_callback(
                            complete_thought,
                            "thinking",
                            "",
                            "",
                        )
                    
                    # Reset for next thinking block
                    current_thinking_chunk = []

            # When agent makes a tool call
            if kind == "on_chat_model_end":
                output = data.get("output")
                if hasattr(output, "tool_calls") and output.tool_calls:
                    for tool_call in output.tool_calls:
                        tool_call_id = tool_call.get("id")
                        tool_name = tool_call.get("name", "Unknown")
                        tool_input = tool_call.get("args", {})

                        # Track tool call in metrics
                        metrics.tool_calls_total.labels(tool_name=tool_name).inc()

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

        if effective_show_thinking:
            # Add accumulated thoughts at the beginning if any
            if thought_buffer:
                # Create a separate thinking step for each thought block
                for i, thought in enumerate(reversed(thought_buffer)):
                    thinking_steps.insert(0, {
                        "thought": thought,
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
        if not self._initialized or self.tools is None:
            return []
        return [tool.name for tool in self.tools]


class AgentManager:
    """Manages multiple SippyAgent instances for different models."""

    def __init__(self, base_config: Config, models_config_path: Optional[str] = None):
        """
        Initialize the AgentManager.
        
        Args:
            base_config: Base configuration with shared settings (API keys, endpoints, etc.)
            models_config_path: Path to models.yaml file
        """
        self.base_config = base_config
        self.agents: Dict[str, SippyAgent] = {}  # Cache of created agents
        self.models: Dict[str, ModelConfig] = {}
        self.default_model_id: str
        
        # Load models configuration
        models_config = load_models_config(models_config_path)
        
        if models_config:
            # Multi-model mode
            for model in models_config["models"]:
                self.models[model.id] = model
            self.default_model_id = models_config["default_model_id"]
            
            if base_config.verbose:
                logger.info(f"Loaded {len(self.models)} models from configuration")
                logger.info(f"Default model: {self.default_model_id}")
        else:
            # Fallback to single-model mode using .env configuration
            # Create a synthetic ModelConfig from the base config
            synthetic_model = ModelConfig(
                id="default",
                name=base_config.model_name,
                description=f"Model from environment configuration",
                model_name=base_config.model_name,
                endpoint=base_config.llm_endpoint,
                temperature=base_config.temperature,
                extended_thinking_budget=base_config.extended_thinking_budget,
                default=True,
            )
            self.models["default"] = synthetic_model
            self.default_model_id = "default"
            
            if base_config.verbose:
                logger.info("No models.yaml found, using single model from environment configuration")
    
    def list_models(self) -> List[Dict[str, Any]]:
        """Get list of available models with their metadata."""
        return [
            {
                "id": model_id,
                "name": model.name,
                "description": model.description,
            }
            for model_id, model in self.models.items()
        ]
    
    def get_default_model_id(self) -> str:
        """Get the ID of the default model."""
        return self.default_model_id
    
    async def get_agent(self, model_id: Optional[str] = None) -> SippyAgent:
        """
        Get or create an agent for the specified model.
        
        Args:
            model_id: ID of the model to use. If None, uses default model.
            
        Returns:
            SippyAgent instance for the specified model.
            
        Raises:
            ValueError: If model_id is not found.
        """
        # Use default model if not specified
        if model_id is None:
            model_id = self.default_model_id
        
        # Check if model exists
        if model_id not in self.models:
            raise ValueError(f"Model '{model_id}' not found. Available models: {list(self.models.keys())}")
        
        # Return cached agent if it exists
        if model_id in self.agents:
            return self.agents[model_id]
        
        # Create new agent
        model_config = self.models[model_id]
        agent_config = model_config.to_config(self.base_config)
        
        if self.base_config.verbose:
            logger.info(f"Creating agent for model: {model_id} ({model_config.name})")
        
        agent = SippyAgent(agent_config)
        # Initialize the agent asynchronously
        await agent._initialize()
        self.agents[model_id] = agent
        
        return agent
    
    def get_model_info(self, model_id: str) -> Optional[ModelConfig]:
        """Get model configuration by ID."""
        return self.models.get(model_id)
