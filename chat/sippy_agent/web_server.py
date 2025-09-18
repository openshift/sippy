"""
FastAPI web server for Sippy Agent.
"""

import asyncio
import json
import logging
from datetime import datetime
from typing import List, Dict, Any, Optional
import uvicorn
from fastapi import FastAPI, WebSocket, WebSocketDisconnect, HTTPException, Depends
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from .agent import SippyAgent
from .config import Config
from .api_models import (
    ChatRequest, ChatResponse, ChatMessage, ThinkingStep,
    StreamMessage, AgentStatus, HealthResponse
)

logger = logging.getLogger(__name__)


class WebSocketManager:
    """Manages WebSocket connections for streaming chat."""
    
    def __init__(self):
        self.active_connections: List[WebSocket] = []
    
    async def connect(self, websocket: WebSocket):
        await websocket.accept()
        self.active_connections.append(websocket)
    
    def disconnect(self, websocket: WebSocket):
        if websocket in self.active_connections:
            self.active_connections.remove(websocket)
    
    async def send_message(self, websocket: WebSocket, message: StreamMessage):
        try:
            await websocket.send_text(message.model_dump_json())
        except Exception as e:
            logger.error(f"Error sending WebSocket message: {e}")
            self.disconnect(websocket)


class SippyWebServer:
    """FastAPI web server for Sippy Agent."""
    
    def __init__(self, config: Config):
        self.config = config
        self.agent = SippyAgent(config)
        self.app = FastAPI(
            title="Sippy AI Agent API",
            description="REST API for Sippy CI/CD Analysis Agent",
            version="1.0.0"
        )
        self.websocket_manager = WebSocketManager()
        self._setup_middleware()
        self._setup_routes()
    
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
            return HealthResponse(
                status="healthy",
                version="1.0.0",
                agent_ready=True
            )
        
        @self.app.get("/status", response_model=AgentStatus)
        async def get_agent_status():
            """Get agent status and configuration."""
            return AgentStatus(
                available_tools=self.agent.list_tools(),
                model_name=self.config.model_name,
                endpoint=self.config.llm_endpoint,
                thinking_enabled=self.config.show_thinking
            )
        
        @self.app.post("/chat", response_model=ChatResponse)
        async def chat(request: ChatRequest):
            """Process a chat message and return the response."""
            try:
                # Convert chat history to context string
                chat_history_context = ""
                if request.chat_history:
                    history_parts = []
                    for msg in request.chat_history[-3:]:  # Last 3 exchanges
                        if msg.role == "user":
                            history_parts.append(f"User: {msg.content}")
                        elif msg.role == "assistant":
                            history_parts.append(f"Assistant: {msg.content}")
                    chat_history_context = "\n".join(history_parts)
                
                # Override thinking setting if provided
                original_thinking = self.config.show_thinking
                if request.show_thinking is not None:
                    self.config.show_thinking = request.show_thinking
                    self.agent.config.show_thinking = request.show_thinking
                
                try:
                    # Process the message
                    result = self.agent.chat(request.message, chat_history_context)
                    
                    if isinstance(result, dict) and "thinking_steps" in result:
                        # Convert thinking steps to API format
                        thinking_steps = []
                        for i, step in enumerate(result["thinking_steps"], 1):
                            thinking_steps.append(ThinkingStep(
                                step_number=i,
                                thought=step.get("thought", ""),
                                action=step.get("action", ""),
                                action_input=step.get("action_input", ""),
                                observation=step.get("observation", "")
                            ))
                        
                        return ChatResponse(
                            response=result["output"],
                            thinking_steps=thinking_steps,
                            tools_used=self._extract_tools_used(result["thinking_steps"])
                        )
                    else:
                        return ChatResponse(
                            response=result,
                            thinking_steps=None,
                            tools_used=None
                        )
                
                finally:
                    # Restore original thinking setting
                    self.config.show_thinking = original_thinking
                    self.agent.config.show_thinking = original_thinking
                
            except Exception as e:
                logger.error(f"Error processing chat request: {e}")
                return ChatResponse(
                    response="I encountered an error while processing your request.",
                    error=str(e)
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
                    
                    # Parse request
                    message = request_data.get("message", "")
                    chat_history = request_data.get("chat_history", [])
                    show_thinking = request_data.get("show_thinking", self.config.show_thinking)
                    
                    # Convert chat history to context
                    chat_history_context = ""
                    if chat_history:
                        history_parts = []
                        for msg in chat_history[-3:]:
                            if msg.get("role") == "user":
                                history_parts.append(f"User: {msg.get('content', '')}")
                            elif msg.get("role") == "assistant":
                                history_parts.append(f"Assistant: {msg.get('content', '')}")
                        chat_history_context = "\n".join(history_parts)
                    
                    # For WebSocket, we'll disable the streaming callback and just send the final result
                    # The real-time streaming is complex to implement correctly with the current agent architecture

                    # Override thinking setting
                    original_thinking = self.config.show_thinking
                    self.config.show_thinking = show_thinking
                    self.agent.config.show_thinking = show_thinking

                    try:
                        # Process message (without streaming for now)
                        result = self.agent.chat(message, chat_history_context)

                        # If thinking was enabled, send thinking steps first
                        if show_thinking and isinstance(result, dict) and "thinking_steps" in result:
                            thinking_steps = result.get("thinking_steps", [])

                            # Send each thinking step
                            for i, step in enumerate(thinking_steps, 1):
                                await self.websocket_manager.send_message(websocket, StreamMessage(
                                    type="thinking_step",
                                    data={
                                        "step_number": i,
                                        "thought": step.get("thought", ""),
                                        "action": step.get("action", ""),
                                        "action_input": step.get("action_input", ""),
                                        "observation": step.get("observation", ""),
                                        "complete": True
                                    }
                                ))
                                # Small delay between steps for better UX
                                await asyncio.sleep(0.1)

                        # Send final response
                        if isinstance(result, dict) and "output" in result:
                            response_text = result["output"]
                            tools_used = self._extract_tools_used(result.get("thinking_steps", []))
                        else:
                            response_text = result
                            tools_used = []

                        await self.websocket_manager.send_message(websocket, StreamMessage(
                            type="final_response",
                            data={
                                "response": response_text,
                                "tools_used": tools_used,
                                "timestamp": datetime.now().isoformat()
                            }
                        ))
                    
                    except Exception as e:
                        logger.error(f"Error in WebSocket chat: {e}")
                        await self.websocket_manager.send_message(websocket, StreamMessage(
                            type="error",
                            data={
                                "error": str(e),
                                "timestamp": datetime.now().isoformat()
                            }
                        ))
                    
                    finally:
                        # Restore original thinking setting
                        self.config.show_thinking = original_thinking
                        self.agent.config.show_thinking = original_thinking
            
            except WebSocketDisconnect:
                self.websocket_manager.disconnect(websocket)
            except Exception as e:
                logger.error(f"WebSocket error: {e}")
                self.websocket_manager.disconnect(websocket)
    
    def _extract_tools_used(self, thinking_steps: List[Dict[str, Any]]) -> List[str]:
        """Extract unique tool names from thinking steps."""
        tools = set()
        for step in thinking_steps:
            action = step.get("action", "")
            if action and action not in ["_Exception", "Invalid", "Error"]:
                tools.add(action)
        return list(tools)
    
    def run(self, host: str = "0.0.0.0", port: int = 8000, reload: bool = False):
        """Run the web server."""
        if reload:
            # For reload mode, use the module path
            uvicorn.run(
                "sippy_agent.web_server:app",
                host=host,
                port=port,
                reload=reload,
                log_level="info"
            )
        else:
            # For non-reload mode, use the app instance directly
            uvicorn.run(
                self.app,
                host=host,
                port=port,
                log_level="info"
            )


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
