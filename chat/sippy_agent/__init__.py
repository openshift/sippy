"""
Sippy AI Agent - A LangChain Re-Act agent for CI analysis.
"""

__version__ = "0.1.0"
__author__ = "Sippy Team"

from .agent import SippyAgent
from .config import Config

__all__ = ["SippyAgent", "Config"]
