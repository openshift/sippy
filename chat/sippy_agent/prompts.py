"""
Prompt template management for Sippy Chat.

This module handles loading and rendering prompt templates from YAML files.
Prompts can be used via REST API or integrated with other systems like MCP.
"""

import logging
from pathlib import Path
from typing import Dict, Any, List, Optional, TypedDict
import yaml
from jinja2 import Environment, BaseLoader, TemplateError, select_autoescape
from jinja2.sandbox import SandboxedEnvironment

logger = logging.getLogger(__name__)


class PromptArgument(TypedDict, total=False):
    """Type definition for a prompt argument."""
    name: str
    description: str
    required: bool
    type: str
    default: Any
    autocomplete: str


class PromptData(TypedDict, total=False):
    """Type definition for a prompt definition."""
    name: str
    description: str
    prompt: str
    arguments: List[PromptArgument]
    hide: bool


def load_prompts_from_directory(prompts_dir: Path) -> Dict[str, PromptData]:
    """
    Load all prompt definitions from YAML files in the prompts directory.
    Supports hierarchical organization with subdirectories.
    
    Args:
        prompts_dir: Path to the directory containing prompt YAML files
        
    Returns:
        Dictionary mapping prompt names to their definitions
    """
    prompts = {}
    
    if not prompts_dir.exists():
        logger.warning(f"Prompts directory not found: {prompts_dir}")
        return prompts
    
    # Recursively find all YAML files in subdirectories
    for yaml_file in prompts_dir.rglob("*.yaml"):
        # Skip example files
        if yaml_file.name.endswith('.example'):
            continue
            
        try:
            with open(yaml_file, "r") as f:
                prompt_data = yaml.safe_load(f)
                
            if not prompt_data or "name" not in prompt_data:
                logger.warning(f"Invalid prompt file {yaml_file}: missing 'name' field")
                continue
                
            prompt_name = prompt_data["name"]
            prompts[prompt_name] = prompt_data
            
            # Get relative path for better logging
            rel_path = yaml_file.relative_to(prompts_dir)
            logger.info(f"Loaded prompt: {prompt_name} from {rel_path}")
            
        except (FileNotFoundError, PermissionError) as e:
            logger.error(f"Cannot access {yaml_file}: {e}")
        except yaml.YAMLError as e:
            logger.error(f"YAML syntax error in {yaml_file}: {e}")
        except Exception as e:
            logger.error(f"Unexpected error loading {yaml_file}: {e}", exc_info=True)
            
    return prompts


def render_prompt(prompt_data: Dict[str, Any], arguments: Dict[str, Any]) -> str:
    """
    Render a prompt template with the provided arguments using Jinja2.
    
    Arguments are merged with their defaults from the prompt definition,
    with provided values taking precedence.
    
    Args:
        prompt_data: The prompt definition from YAML
        arguments: Dictionary of argument values to substitute
        
    Returns:
        Rendered prompt text
    """
    # Get prompt content
    content = prompt_data.get("prompt", "")
    
    if not content:
        logger.warning("No prompt content found in prompt data")
        return ""
    
    # Build a map of argument defaults from the arguments section
    arg_defaults = {}
    for arg_def in prompt_data.get("arguments", []):
        if "default" in arg_def:
            arg_defaults[arg_def["name"]] = arg_def["default"]
    
    # Merge provided arguments with defaults
    # Provided arguments take precedence over defaults
    template_vars = {}
    for arg_def in prompt_data.get("arguments", []):
        arg_name = arg_def["name"]
        if arg_name in arguments and arguments[arg_name] is not None:
            # Use provided value
            template_vars[arg_name] = arguments[arg_name]
        elif arg_name in arg_defaults:
            # Use default value
            template_vars[arg_name] = arg_defaults[arg_name]
        # If neither provided nor has default, variable won't be in template_vars
    
    # Render using Jinja2 with security measures:
    # - SandboxedEnvironment prevents arbitrary code execution in templates
    # - Restricts access to Python internals and dangerous operations
    # - autoescape prevents XSS when rendering user-provided content
    try:
        env = SandboxedEnvironment(
            loader=BaseLoader(),
            autoescape=select_autoescape(default=True)
        )
        template = env.from_string(content)
        rendered = template.render(**template_vars)
        return rendered
    except TemplateError as e:
        logger.error(f"Jinja2 template error: {e}")
        raise ValueError(f"Failed to render prompt template: {e}")


def get_default_prompts_dir() -> Path:
    """Get the default prompts directory path."""
    chat_dir = Path(__file__).parent.parent
    return chat_dir / "prompts"


class PromptManager:
    """Manages prompt templates for Sippy Chat."""
    
    def __init__(self, prompts_dir: Optional[Path] = None):
        """
        Initialize the prompt manager.
        
        Args:
            prompts_dir: Optional path to prompts directory. 
                        If None, uses default location.
        """
        if prompts_dir is None:
            prompts_dir = get_default_prompts_dir()
        
        self.prompts_dir = prompts_dir
        self.prompts = load_prompts_from_directory(prompts_dir)
        logger.info(f"Loaded {len(self.prompts)} prompts from {prompts_dir}")
    
    def list_prompts(self) -> List[Dict[str, Any]]:
        """
        Get a list of all available prompts.
        
        Returns:
            List of prompt metadata (name, description, arguments, hide)
        """
        return [
            {
                "name": name,
                "description": data.get("description", ""),
                "arguments": data.get("arguments", []),
                "hide": data.get("hide", False),
            }
            for name, data in self.prompts.items()
        ]
    
    def get_prompt(self, name: str) -> Optional[Dict[str, Any]]:
        """
        Get a prompt definition by name.
        
        Args:
            name: The prompt name
            
        Returns:
            The prompt definition or None if not found
        """
        return self.prompts.get(name)
    
    def render(self, name: str, arguments: Dict[str, Any]) -> Optional[str]:
        """
        Render a prompt with the given arguments.
        
        Args:
            name: The prompt name
            arguments: Dictionary of argument values
            
        Returns:
            Rendered prompt text or None if prompt not found
        """
        prompt_data = self.get_prompt(name)
        if not prompt_data:
            return None
        
        return render_prompt(prompt_data, arguments)
    
    def reload(self):
        """Reload all prompts from disk."""
        self.prompts = load_prompts_from_directory(self.prompts_dir)
        logger.info(f"Reloaded {len(self.prompts)} prompts from {self.prompts_dir}")
