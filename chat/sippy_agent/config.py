"""
Configuration management for Sippy Agent.
"""

import os
import yaml
from pathlib import Path
from typing import Optional, List, Dict, Any
from pydantic import BaseModel, Field
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()


class ModelConfig(BaseModel):
    """Configuration for a single model."""

    id: str = Field(description="Unique identifier for the model")
    name: str = Field(description="Display name for the model")
    description: Optional[str] = Field(default=None, description="Description of the model")
    model_name: str = Field(description="The actual model name to use with the provider")
    endpoint: str = Field(default="", description="API endpoint URL (empty for Vertex AI)")
    temperature: Optional[float] = Field(default=None, description="Temperature setting for the model")
    extended_thinking_budget: Optional[int] = Field(default=None, description="Token budget for Claude's extended thinking")
    default: bool = Field(default=False, description="Whether this is the default model")

    def to_config(self, base_config: "Config") -> "Config":
        """Convert ModelConfig to a Config object, inheriting base settings."""
        config_dict = base_config.model_dump()
        
        # Override with model-specific settings
        config_dict["model_name"] = self.model_name
        config_dict["llm_endpoint"] = self.endpoint if self.endpoint else base_config.llm_endpoint
        
        # Only override temperature and extended_thinking_budget if explicitly set
        if self.temperature is not None:
            config_dict["temperature"] = self.temperature
        if self.extended_thinking_budget is not None:
            config_dict["extended_thinking_budget"] = self.extended_thinking_budget
        
        return Config(**config_dict)


class Config(BaseModel):
    """Configuration settings for the Sippy Agent."""

    # LLM Configuration
    llm_endpoint: str = Field(
        default_factory=lambda: os.getenv("LLM_ENDPOINT", "http://localhost:11434/v1"), description="LLM API endpoint (OpenAI compatible)"
    )

    openai_api_key: Optional[str] = Field(
        default_factory=lambda: os.getenv("OPENAI_API_KEY"), description="OpenAI API key (optional, not needed for local endpoints)"
    )

    google_api_key: Optional[str] = Field(
        default_factory=lambda: os.getenv("GOOGLE_API_KEY"), description="Google API key for Gemini models (required when using Gemini)"
    )

    google_credentials_file: Optional[str] = Field(
        default_factory=lambda: os.getenv("GOOGLE_APPLICATION_CREDENTIALS"),
        description="Path to Google service account credentials JSON file (alternative to API key)",
    )

    google_project_id: Optional[str] = Field(
        default_factory=lambda: os.getenv("GOOGLE_PROJECT_ID"),
        description="Google Cloud project ID for Vertex AI (required when using Claude models via Vertex AI)",
    )

    google_location: str = Field(
        default_factory=lambda: os.getenv("GOOGLE_LOCATION", "us-central1"),
        description="Google Cloud location/region for Vertex AI (default: us-central1)",
    )

    model_name: str = Field(
        default_factory=lambda: os.getenv("MODEL_NAME", "llama3.1:8b"),
        description="Model name to use (e.g., llama3.1:8b for Ollama, gpt-4 for OpenAI)",
    )

    # Sippy API Configuration (for future use)
    sippy_api_url: Optional[str] = Field(default_factory=lambda: os.getenv("SIPPY_API_URL"), description="Base URL for the Sippy API")

    # Jira Configuration
    jira_url: str = Field(default_factory=lambda: os.getenv("JIRA_URL", "https://issues.redhat.com"), description="Jira instance URL")

    jira_username: Optional[str] = Field(
        default_factory=lambda: os.getenv("JIRA_USERNAME"), description="Jira username for authentication (optional for public queries)"
    )

    jira_token: Optional[str] = Field(
        default_factory=lambda: os.getenv("JIRA_TOKEN"), description="Jira API token for authentication (optional for public queries)"
    )

    # MCP Configuration
    mcp_config_file: Optional[str] = Field(
        default_factory=lambda: os.getenv("MCP_CONFIG_FILE"), description="Path to the MCP servers JSON configuration file"
    )

    # Database Configuration
    sippy_ro_database_dsn: Optional[str] = Field(
        default_factory=lambda: os.getenv("SIPPY_READ_ONLY_DATABASE_DSN"),
        description="PostgreSQL connection string for read-only database access (e.g., postgresql://user:pass@host:5432/dbname)"
    )

    # Agent Configuration
    max_iterations: int = Field(default=15, description="Maximum number of iterations for the Re-Act agent")

    max_execution_time: int = Field(
        default=1800, description="Maximum execution time in seconds for the agent (default: 1800 = 30 minutes)"
    )

    verbose: bool = Field(default=False, description="Enable verbose logging")

    temperature: float = Field(default=0.0, description="Temperature setting for the language model")

    show_thinking: bool = Field(default=False, description="Show the agent's thinking process (thoughts, actions, observations)")

    extended_thinking_budget: int = Field(
        default_factory=lambda: int(os.getenv("EXTENDED_THINKING_BUDGET", "10000")),
        description="Token budget for Claude's extended thinking feature (default: 10000)"
    )

    persona: str = Field(default_factory=lambda: os.getenv("PERSONA", "default"), description="AI persona to use (default, zorp, etc.)")

    def is_openai_endpoint(self) -> bool:
        """Check if the endpoint is OpenAI's API."""
        return "openai.com" in self.llm_endpoint.lower()

    def is_local_endpoint(self) -> bool:
        """Check if the endpoint is a local endpoint."""
        return "localhost" in self.llm_endpoint or "127.0.0.1" in self.llm_endpoint

    def is_gemini_model(self) -> bool:
        """Check if the model is a Gemini model."""
        return self.model_name.startswith("gemini")

    def is_claude_model(self) -> bool:
        """Check if the model is a Claude model (via Vertex AI)."""
        return self.model_name.startswith("claude")

    def validate_required_settings(self) -> None:
        """Validate that required settings are present."""
        # Only require OpenAI API key if using OpenAI's endpoint
        if self.is_openai_endpoint() and not self.openai_api_key:
            raise ValueError(
                "OpenAI API key is required when using OpenAI endpoint. Set OPENAI_API_KEY environment variable or use a local endpoint."
            )

        # Require Google API key or credentials file if using Gemini models
        if self.is_gemini_model() and not self.google_api_key and not self.google_credentials_file:
            raise ValueError(
                "Google API key or service account credentials file is required when using Gemini models. "
                "Set GOOGLE_API_KEY environment variable or GOOGLE_APPLICATION_CREDENTIALS file path."
            )

        # Require Google project ID for Claude models via Vertex AI
        # Credentials can come from either explicit file or gcloud auth (ADC)
        if self.is_claude_model():
            if not self.google_project_id:
                raise ValueError(
                    "Google Cloud project ID is required when using Claude models via Vertex AI. "
                    "Set GOOGLE_PROJECT_ID environment variable."
                )

    @classmethod
    def from_env(cls) -> "Config":
        """Create configuration from environment variables."""
        config = cls()
        config.validate_required_settings()
        return config


def load_models_config(config_path: Optional[str] = None) -> Optional[Dict[str, Any]]:
    """
    Load models configuration from YAML file.
    
    Args:
        config_path: Path to models.yaml file. If None, looks for models.yaml in current directory.
        
    Returns:
        Dictionary with 'models' list and 'default_model_id', or None if file doesn't exist.
    """
    if config_path is None:
        # Look for models.yaml in the chat directory
        config_path = Path(__file__).parent.parent / "models.yaml"
    else:
        config_path = Path(config_path)
    
    if not config_path.exists():
        return None
    
    try:
        with open(config_path, 'r') as f:
            data = yaml.safe_load(f)
        
        if not data or 'models' not in data:
            raise ValueError("models.yaml must contain a 'models' key with a list of models")
        
        models = []
        default_model_id = None
        
        for model_data in data['models']:
            model = ModelConfig(**model_data)
            models.append(model)
            
            if model.default:
                if default_model_id is not None:
                    raise ValueError(f"Multiple default models found: {default_model_id} and {model.id}")
                default_model_id = model.id
        
        if not models:
            raise ValueError("models.yaml must contain at least one model")
        
        # If no default specified, use the first model
        if default_model_id is None:
            default_model_id = models[0].id
        
        return {
            "models": models,
            "default_model_id": default_model_id
        }
    
    except Exception as e:
        raise ValueError(f"Error loading models configuration: {e}")
