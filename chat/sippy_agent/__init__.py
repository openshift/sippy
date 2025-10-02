"""
Sippy AI Agent - A LangGraph ReAct agent for CI analysis.
"""

__version__ = "0.2.0"
__author__ = "Technical Release Team"

from .agent import SippyAgent
from .config import Config

__all__ = ["SippyAgent", "Config"]
