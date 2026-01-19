"""
FastAPI web server for Sippy Agent.
"""

import asyncio
import json
import logging
import re
import time
from datetime import datetime
from typing import List, Dict, Any, Optional
import uvicorn
from fastapi import FastAPI, WebSocket, WebSocketDisconnect, Response
from fastapi.middleware.cors import CORSMiddleware
from prometheus_client import generate_latest, CONTENT_TYPE_LATEST

from .agent import SippyAgent, AgentManager
from .config import Config
from .api_models import (
    ChatRequest,
    ChatResponse,
    ChatMessage,
    ThinkingStep,
    StreamMessage,
    AgentStatus,
    HealthResponse,
    PersonaInfo,
    PersonasResponse,
    ModelInfo,
    ModelsResponse,
    Visualization,
)
from . import metrics
from .metrics_server import start_metrics_server, stop_metrics_server
from .prompts import PromptManager, render_prompt

logger = logging.getLogger(__name__)


class WebSocketManager:
    """Manages WebSocket connections for streaming chat."""

    def __init__(self):
        self.active_connections: List[WebSocket] = []
        self.active_tasks: Dict[WebSocket, asyncio.Task] = {}

    async def connect(self, websocket: WebSocket):
        await websocket.accept()
        self.active_connections.append(websocket)
        # Track active sessions
        metrics.active_sessions.inc()
        metrics.sessions_started_total.inc()

    def disconnect(self, websocket: WebSocket):
        if websocket in self.active_connections:
            self.active_connections.remove(websocket)
            # Track active sessions
            metrics.active_sessions.dec()
        
        # Cancel any active task for this websocket
        if websocket in self.active_tasks:
            task = self.active_tasks[websocket]
            if not task.done():
                task.cancel()
                logger.info("Cancelled active task for disconnected websocket")
            del self.active_tasks[websocket]

    def set_active_task(self, websocket: WebSocket, task: asyncio.Task):
        """Track the active task for a websocket connection."""
        self.active_tasks[websocket] = task

    def clear_active_task(self, websocket: WebSocket):
        """Clear the active task for a websocket connection."""
        if websocket in self.active_tasks:
            del self.active_tasks[websocket]

    async def send_message(self, websocket: WebSocket, message: StreamMessage):
        try:
            await websocket.send_text(message.model_dump_json())
        except Exception as e:
            logger.error(f"Error sending WebSocket message: {e}")
            self.disconnect(websocket)


class SippyWebServer:
    """FastAPI web server for Sippy Agent."""

    def __init__(self, config: Config, metrics_port: Optional[int] = None, models_config_path: Optional[str] = None):
        self.config = config
        self.metrics_port = metrics_port
        self.agent_manager = AgentManager(config, models_config_path)
        self.app = FastAPI(
            title="Sippy AI Agent API",
            description="REST API for Sippy CI/CD Analysis Agent",
            version="1.0.0",
        )
        self.websocket_manager = WebSocketManager()
        
        # Initialize prompt manager
        self.prompt_manager = PromptManager()
        
        self._setup_middleware()
        self._setup_routes()
        
        # Initialize agent info metrics
        metrics.agent_info.info({
            "version": "1.0.0",
            "model": config.model_name,
            "endpoint": config.llm_endpoint,
            "persona": config.persona,
        })

    def _setup_middleware(self):
        """Setup CORS and other middleware."""
        self.app.add_middleware(
            CORSMiddleware,
            allow_origins=["*"],  # Configure this for production
            allow_credentials=True,
            allow_methods=["*"],
            allow_headers=["*"],
        )

    def _setup_routes(self):
        """Setup API routes."""

        @self.app.get("/health", response_model=HealthResponse)
        async def health_check():
            """Health check endpoint."""
            return HealthResponse(status="healthy", version="1.0.0", agent_ready=True)

        @self.app.get("/metrics")
        async def prometheus_metrics():
            """Prometheus metrics endpoint."""
            return Response(
                content=generate_latest(),
                media_type=CONTENT_TYPE_LATEST
            )

        @self.app.get("/chat/models", response_model=ModelsResponse)
        async def get_models():
            """Get list of available models."""
            models_list = self.agent_manager.list_models()
            return ModelsResponse(
                models=[ModelInfo(**model) for model in models_list],
                default_model=self.agent_manager.get_default_model_id(),
            )

        @self.app.get("/status", response_model=AgentStatus)
        async def get_agent_status():
            """Get agent status and configuration."""
            from .personas import list_persona_names

            # Get default agent for status info
            default_agent = await self.agent_manager.get_agent()
            
            return AgentStatus(
                available_tools=default_agent.list_tools(),
                model_name=self.config.model_name,
                endpoint=self.config.llm_endpoint,
                thinking_enabled=self.config.show_thinking,
                current_persona=self.config.persona,
                available_personas=list_persona_names(),
            )

        @self.app.get("/chat/personas", response_model=PersonasResponse)
        async def get_personas():
            """Get list of available personas."""
            from .personas import PERSONAS

            personas = [
                PersonaInfo(
                    name=name,
                    description=p.description,
                    style_instructions=p.style_instructions,
                )
                for name, p in PERSONAS.items()
            ]

            return PersonasResponse(
                personas=personas, current_persona=self.config.persona
            )

        @self.app.get("/chat/prompts")
        async def get_prompts():
            """Get list of available prompt templates."""
            return {"prompts": self.prompt_manager.list_prompts()}

        @self.app.post("/chat/prompts/render")
        async def render_prompt_endpoint(request: dict):
            """Render a prompt template with provided arguments."""
            prompt_name = request.get("prompt_name")
            arguments = request.get("arguments", {})
            
            if not prompt_name:
                return {"error": "prompt_name is required"}, 400
            
            prompt_data = self.prompt_manager.get_prompt(prompt_name)
            if not prompt_data:
                return {"error": f"Prompt '{prompt_name}' not found"}, 404
            
            # Use the render_prompt function from prompts module
            rendered_text = render_prompt(prompt_data, arguments)
            
            return {
                "rendered": rendered_text
            }

        @self.app.post("/chat", response_model=ChatResponse)
        async def chat(request: ChatRequest):
            """Process a chat message and return the response."""
            # Track message received
            metrics.messages_received_total.labels(endpoint="http").inc()

            # Track request message size
            request_size = len(request.message.encode('utf-8'))
            metrics.message_size_bytes.labels(direction="request").observe(request_size)

            start_time = time.time()
            try:
                # Get the appropriate agent for the requested model
                agent = await self.agent_manager.get_agent(request.model_id)
                # Determine which model is actually being used
                model_id = request.model_id or self.agent_manager.get_default_model_id()

                # Process the message with request-specific persona and show_thinking
                # These are passed as parameters, not mutating the shared config
                result = await agent.achat(
                    request.message,
                    request.chat_history,
                    persona=request.persona,
                    show_thinking=request.show_thinking,
                )

                # Process the response using common method
                processed = self._process_agent_response(result)

                # Track response duration
                duration = time.time() - start_time
                metrics.response_duration_seconds.labels(endpoint="http").observe(duration)

                return ChatResponse(
                    response=processed["response_text"],
                    thinking_steps=processed["thinking_steps"],
                    tools_used=processed["tools_used"],
                    visualizations=processed["visualizations"],
                    model_id=model_id,
                )

            except Exception as e:
                logger.error(f"Error processing chat request: {e}")
                metrics.errors_total.labels(error_type="processing_error").inc()
                return ChatResponse(
                    response="I encountered an error while processing your request.",
                    error=str(e),
                )

        @self.app.websocket("/chat/stream")
        async def websocket_chat(websocket: WebSocket):
            """WebSocket endpoint for streaming chat with real-time thinking."""
            await self.websocket_manager.connect(websocket)

            try:
                while True:
                    # Receive message from client
                    data = await websocket.receive_text()
                    request_data = json.loads(data)
                    
                    # Track message received
                    metrics.messages_received_total.labels(endpoint="websocket").inc()

                    # Parse request
                    message = request_data.get("message", "")
                    
                    # Track request message size
                    request_size = len(message.encode('utf-8'))
                    metrics.message_size_bytes.labels(direction="request").observe(request_size)
                    chat_history_data = request_data.get("chat_history", [])
                    chat_history = [ChatMessage(**msg) for msg in chat_history_data]
                    show_thinking = request_data.get(
                        "show_thinking", self.config.show_thinking
                    )
                    persona = request_data.get("persona", self.config.persona)
                    model_id = request_data.get("model_id")
                    page_context = request_data.get("page_context")

                    logger.info(f"Received page context: {page_context}")
                    
                    # Start timing the response
                    start_time = time.time()
                    
                    # Get the appropriate agent for the requested model
                    try:
                        agent = await self.agent_manager.get_agent(model_id)
                    except Exception as e:
                        logger.error(f"Error getting agent for model '{model_id}': {e}")
                        metrics.errors_total.labels(error_type="agent_initialization_error").inc()
                        await self.websocket_manager.send_message(
                            websocket,
                            StreamMessage(
                                type="error",
                                data={
                                    "error": str(e),
                                    "timestamp": datetime.now().isoformat(),
                                },
                            ),
                        )
                        continue
                    
                    # Determine which model is actually being used
                    actual_model_id = model_id or self.agent_manager.get_default_model_id()

                    async def process_message():
                        """Process the message - can be cancelled if websocket disconnects."""
                        try:
                            # Track step number for streaming
                            step_counter = {"count": 0}
                            # Map tool calls to their step numbers for parallel execution
                            tool_call_steps = {}

                            # Define async thinking callback for real-time streaming
                            async def thinking_callback(
                                thought: str,
                                action: str,
                                action_input: str,
                                observation: str,
                            ):
                                """Stream thinking steps in real-time over WebSocket."""
                                # For "thinking" actions (Gemini thoughts), mark as complete immediately
                                # For tool calls, only complete when we have an observation
                                is_complete = action == "thinking" or bool(observation)
                                
                                # Create a unique key for this tool call based on action and input
                                tool_key = f"{action}:{action_input}"

                                # Only increment step counter on new calls (no observation yet)
                                if not observation:
                                    step_counter["count"] += 1
                                    current_step = step_counter["count"]
                                    # Store the step number for this call
                                    tool_call_steps[tool_key] = current_step
                                else:
                                    # Retrieve the step number for this call
                                    current_step = tool_call_steps.get(
                                        tool_key, step_counter["count"]
                                    )

                                # Send the thinking step immediately
                                await self.websocket_manager.send_message(
                                    websocket,
                                    StreamMessage(
                                        type="thinking_step",
                                        data={
                                            "step_number": current_step,
                                            "thought": thought,
                                            "action": action,
                                            "action_input": action_input,
                                            "observation": observation,
                                            "complete": is_complete,
                                        },
                                    ),
                                )

                            # Enhance message with page context if provided
                            enhanced_message = message
                            if page_context:
                                context_str = self._format_page_context(page_context)
                                enhanced_message = (
                                    f"{context_str}\n\nUser question: {message}"
                                )
                                logger.info(
                                    f"Enhanced message with context: {enhanced_message[:200]}..."
                                )

                            # Process message with streaming callback
                            # Pass persona and show_thinking as parameters, not mutating config
                            result = await agent.achat(
                                enhanced_message,
                                chat_history,
                                thinking_callback=(
                                    thinking_callback if show_thinking else None
                                ),
                                persona=persona,
                                show_thinking=show_thinking,
                            )

                            # Process the response using common method
                            processed = self._process_agent_response(result)

                            await self.websocket_manager.send_message(
                                websocket,
                                StreamMessage(
                                    type="final_response",
                                    data={
                                        "response": processed["response_text"],
                                        "tools_used": processed["tools_used"],
                                        "visualizations": [
                                            v.model_dump() for v in processed["visualizations"]
                                        ] if processed["visualizations"] else [],
                                        "model_id": actual_model_id,
                                        "timestamp": datetime.now().isoformat(),
                                    },
                                ),
                            )

                        except asyncio.CancelledError:
                            logger.info("Message processing cancelled by client")
                            metrics.cancelled_requests_total.labels(endpoint="websocket").inc()
                            raise
                        except Exception as e:
                            logger.error(f"Error in WebSocket chat: {e}")
                            metrics.errors_total.labels(error_type="agent_error").inc()
                            await self.websocket_manager.send_message(
                                websocket,
                                StreamMessage(
                                    type="error",
                                    data={
                                        "error": str(e),
                                        "timestamp": datetime.now().isoformat(),
                                    },
                                ),
                            )

                    try:
                        # Create and track the processing task
                        task = asyncio.create_task(process_message())
                        self.websocket_manager.set_active_task(websocket, task)
                        
                        # Wait for the task to complete
                        await task

                    except asyncio.CancelledError:
                        logger.info("Task cancelled, client stopped generation")
                        # Task was cancelled, which is fine
                        pass
                    finally:
                        # Clear the task tracking
                        self.websocket_manager.clear_active_task(websocket)

                        # Track response duration
                        duration = time.time() - start_time
                        metrics.response_duration_seconds.labels(endpoint="websocket").observe(duration)

            except WebSocketDisconnect:
                self.websocket_manager.disconnect(websocket)
            except Exception as e:
                logger.error(f"WebSocket error: {e}")
                metrics.errors_total.labels(error_type="websocket_error").inc()
                self.websocket_manager.disconnect(websocket)

    def _format_page_context(self, page_context: Dict[str, Any]) -> str:
        """Format page context as JSON for the agent."""
        if not page_context:
            return ""

        # Extract special fields
        instructions = page_context.get("instructions", "")

        # Create a copy without instructions and suggestedQuestions for the data section
        data_context = {
            k: v
            for k, v in page_context.items()
            if k not in ["instructions", "suggestedQuestions"]
        }

        context_str = "[Current Page Context]\n"
        context_str += "The user is viewing the following page. Use this context to better answer their question:\n\n"
        context_str += json.dumps(data_context, indent=2)

        # Append page-specific instructions if present
        if instructions:
            context_str += "\n\n[Page-Specific Instructions]\n"
            context_str += instructions

        return context_str

    def _extract_tools_used(self, thinking_steps: List[Dict[str, Any]]) -> List[str]:
        """Extract unique tool names from thinking steps."""
        tools = set()
        for step in thinking_steps:
            action = step.get("action", "")
            if action and action not in ["_Exception", "Invalid", "Error"]:
                tools.add(action)
        return list(tools)

    def _extract_visualizations_from_text(self, text: str) -> List[Visualization]:
        """Extract visualization specifications from text content.

        Looks for JSON blocks between VISUALIZATION_START and VISUALIZATION_END markers.
        """
        visualizations = []

        if not text or not isinstance(text, str):
            return visualizations

        # Find all visualization blocks in the text
        start_marker = "VISUALIZATION_START"
        end_marker = "VISUALIZATION_END"

        current_pos = 0
        while True:
            start_idx = text.find(start_marker, current_pos)
            if start_idx == -1:
                break

            end_idx = text.find(end_marker, start_idx)
            if end_idx == -1:
                logger.warning("Found VISUALIZATION_START without matching VISUALIZATION_END")
                break

            try:
                # Extract JSON between markers
                viz_start = start_idx + len(start_marker)
                viz_json = text[viz_start:end_idx].strip()

                # Parse the JSON
                viz_data = json.loads(viz_json)

                # Get layout and add AI-generated annotation
                layout = viz_data.get("layout", {})
                
                # Ensure top margin is sufficient for the title and subtitle
                if "margin" not in layout:
                    layout["margin"] = {}
                if "t" not in layout["margin"] or layout["margin"]["t"] < 80:
                    layout["margin"]["t"] = 80
                
                # Add AI-generated caption as an annotation below the title
                if "annotations" not in layout:
                    layout["annotations"] = []
                
                # Position the caption in the margin area, closer to the title
                # y > 1.0 places it in the top margin area
                layout["annotations"].append({
                    "text": "<i>Generated with AI by Sippy Chat</i>",
                    "xref": "paper",
                    "yref": "paper",
                    "x": 0.5,
                    "y": 1.00,  # Just above the plot area in the margin
                    "xanchor": "center",
                    "yanchor": "bottom",
                    "showarrow": False,
                    "font": {"size": 10, "color": "#666666"}
                })

                # Create Visualization object
                visualization = Visualization(
                    data=viz_data.get("data", []),
                    layout=layout,
                    config=viz_data.get("config"),
                )
                visualizations.append(visualization)

                logger.info(f"Extracted visualization from response text")
            except (json.JSONDecodeError, ValueError, KeyError) as e:
                logger.warning(f"Failed to parse visualization: {e}")

            # Move past this visualization block
            current_pos = end_idx + len(end_marker)

        return visualizations

    def _extract_visualizations(self, response_text: str) -> List[Visualization]:
        """Extract visualizations from response text only (not from tool observations)."""
        visualizations = []

        # Extract from main response text only
        if response_text:
            visualizations.extend(self._extract_visualizations_from_text(response_text))

        return visualizations

    def _strip_visualization_markers(self, text: str) -> str:
        """Remove VISUALIZATION_START...VISUALIZATION_END blocks from text."""
        if not text or not isinstance(text, str):
            return text
        
        # Remove all visualization blocks (non-greedy match)
        cleaned = re.sub(
            r'VISUALIZATION_START[\s\S]*?VISUALIZATION_END',
            '',
            text,
            flags=re.MULTILINE
        )
        return cleaned.strip()

    def _process_agent_response(self, result: Any) -> Dict[str, Any]:
        """
        Process agent response and extract all components.
        
        Args:
            result: The result from agent.achat() - can be dict with thinking_steps or simple string
            
        Returns:
            Dict containing: response_text, thinking_steps (API format), tools_used, visualizations
        """
        if isinstance(result, dict) and "thinking_steps" in result:
            # Response with thinking steps
            response_text = result["output"]
            thinking_steps = result["thinking_steps"]
            tools_used = self._extract_tools_used(thinking_steps)
            
            # Convert thinking steps to API format
            api_thinking_steps = []
            for i, step in enumerate(thinking_steps, 1):
                api_thinking_steps.append(
                    ThinkingStep(
                        step_number=i,
                        thought=step.get("thought", ""),
                        action=step.get("action", ""),
                        action_input=step.get("action_input", ""),
                        observation=step.get("observation", ""),
                    )
                )
            thinking_steps = api_thinking_steps
        else:
            # Simple response without thinking steps
            response_text = result
            thinking_steps = None
            tools_used = []
        
        # Track response size metrics
        response_size = len(response_text.encode('utf-8'))
        metrics.message_size_bytes.labels(direction="response").observe(response_size)
        
        # Extract visualizations and strip markers from response
        visualizations = self._extract_visualizations(response_text)
        clean_response = self._strip_visualization_markers(response_text)
        
        return {
            "response_text": clean_response,
            "thinking_steps": thinking_steps,
            "tools_used": tools_used,
            "visualizations": visualizations or None,
        }

    def run(self, host: str = "0.0.0.0", port: int = 8000, reload: bool = False):
        """Run the web server."""
        # Start separate metrics server if port is specified
        if self.metrics_port:
            logger.info(f"Starting metrics server on port {self.metrics_port}")
            start_metrics_server(host="0.0.0.0", port=self.metrics_port)
        
        try:
            if reload:
                # For reload mode, use the module path
                uvicorn.run(
                    "sippy_agent.web_server:app",
                    host=host,
                    port=port,
                    reload=reload,
                    log_level="info",
                )
            else:
                # For non-reload mode, use the app instance directly
                uvicorn.run(self.app, host=host, port=port, log_level="info")
        finally:
            # Stop metrics server on shutdown
            if self.metrics_port:
                stop_metrics_server()


# Global app instance for uvicorn - initialized lazily
app = None


def get_app() -> FastAPI:
    """Get or create the FastAPI app instance."""
    global app
    if app is None:
        config = Config.from_env()
        server = SippyWebServer(config)
        app = server.app
    return app


# Initialize the app for uvicorn
app = get_app()
