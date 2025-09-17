"""
Configuration management for Sippy Agent.
"""

import os
from typing import Optional
from pydantic import BaseModel, Field
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()


class Config(BaseModel):
    """Configuration settings for the Sippy Agent."""

    # LLM Configuration
    llm_endpoint: str = Field(
        default_factory=lambda: os.getenv("LLM_ENDPOINT", "http://localhost:11434/v1"),
        description="LLM API endpoint (OpenAI compatible)"
    )

    openai_api_key: Optional[str] = Field(
        default_factory=lambda: os.getenv("OPENAI_API_KEY"),
        description="OpenAI API key (optional, not needed for local endpoints)"
    )

    google_api_key: Optional[str] = Field(
        default_factory=lambda: os.getenv("GOOGLE_API_KEY"),
        description="Google API key for Gemini models (required when using Gemini)"
    )

    google_credentials_file: Optional[str] = Field(
        default_factory=lambda: os.getenv("GOOGLE_APPLICATION_CREDENTIALS"),
        description="Path to Google service account credentials JSON file (alternative to API key)"
    )

    model_name: str = Field(
        default_factory=lambda: os.getenv("MODEL_NAME", "llama3.1:8b"),
        description="Model name to use (e.g., llama3.1:8b for Ollama, gpt-4 for OpenAI)"
    )

    # Sippy API Configuration (for future use)
    sippy_api_url: Optional[str] = Field(
        default_factory=lambda: os.getenv("SIPPY_API_URL"),
        description="Base URL for the Sippy API"
    )

    # Jira Configuration
    jira_url: str = Field(
        default_factory=lambda: os.getenv("JIRA_URL", "https://issues.redhat.com"),
        description="Jira instance URL"
    )

    jira_username: Optional[str] = Field(
        default_factory=lambda: os.getenv("JIRA_USERNAME"),
        description="Jira username for authentication (optional for public queries)"
    )

    jira_token: Optional[str] = Field(
        default_factory=lambda: os.getenv("JIRA_TOKEN"),
        description="Jira API token for authentication (optional for public queries)"
    )
    
    # Agent Configuration
    max_iterations: int = Field(
        default=15,
        description="Maximum number of iterations for the Re-Act agent"
    )

    max_execution_time: int = Field(
        default=1800,
        description="Maximum execution time in seconds for the agent (default: 1800 = 30 minutes)"
    )
    
    verbose: bool = Field(
        default=False,
        description="Enable verbose logging"
    )
    
    temperature: float = Field(
        default=0.0,
        description="Temperature setting for the language model"
    )

    show_thinking: bool = Field(
        default=False,
        description="Show the agent's thinking process (thoughts, actions, observations)"
    )
    
    def is_openai_endpoint(self) -> bool:
        """Check if the endpoint is OpenAI's API."""
        return "openai.com" in self.llm_endpoint.lower()

    def is_local_endpoint(self) -> bool:
        """Check if the endpoint is a local endpoint."""
        return "localhost" in self.llm_endpoint or "127.0.0.1" in self.llm_endpoint

    def is_gemini_model(self) -> bool:
        """Check if the model is a Gemini model."""
        return self.model_name.startswith("gemini")

    def validate_required_settings(self) -> None:
        """Validate that required settings are present."""
        # Only require OpenAI API key if using OpenAI's endpoint
        if self.is_openai_endpoint() and not self.openai_api_key:
            raise ValueError(
                "OpenAI API key is required when using OpenAI endpoint. "
                "Set OPENAI_API_KEY environment variable or use a local endpoint."
            )

        # Require Google API key or credentials file if using Gemini models
        if self.is_gemini_model() and not self.google_api_key and not self.google_credentials_file:
            raise ValueError(
                "Google API key or service account credentials file is required when using Gemini models. "
                "Set GOOGLE_API_KEY environment variable or GOOGLE_APPLICATION_CREDENTIALS file path."
            )
    
    @classmethod
    def from_env(cls) -> "Config":
        """Create configuration from environment variables."""
        config = cls()
        config.validate_required_settings()
        return config
