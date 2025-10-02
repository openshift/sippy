"""
LangGraph ReAct agent implementation for Sippy.
"""

import logging
from typing import TypedDict, Annotated, List, Union, Literal
from langchain_core.messages import BaseMessage, HumanMessage, AIMessage, ToolMessage
from langchain_core.prompts import ChatPromptTemplate, MessagesPlaceholder
from langchain_openai import ChatOpenAI
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain.tools import BaseTool
from langgraph.graph import StateGraph, END
from langgraph.prebuilt import ToolNode
from langgraph.graph.message import add_messages

logger = logging.getLogger(__name__)


# Define the agent state
class AgentState(TypedDict):
    """The state of the agent."""

    messages: Annotated[List[BaseMessage], add_messages]
    # Track iterations to prevent infinite loops
    iterations: int


def create_react_graph(llm: Union[ChatOpenAI, ChatGoogleGenerativeAI], tools: List[BaseTool], system_prompt: str, max_iterations: int = 15):
    """
    Create a ReAct agent graph using LangGraph.

    Args:
        llm: The language model to use
        tools: List of tools available to the agent
        system_prompt: System prompt for the agent
        max_iterations: Maximum number of reasoning iterations

    Returns:
        A compiled LangGraph
    """

    # Bind tools to the LLM
    llm_with_tools = llm.bind_tools(tools)

    # Create the prompt template
    prompt = ChatPromptTemplate.from_messages(
        [
            ("system", system_prompt),
            MessagesPlaceholder(variable_name="messages"),
        ]
    )

    # Define the agent node
    def call_model(state: AgentState) -> AgentState:
        """Call the model to decide what to do next."""
        messages = state["messages"]
        iterations = state.get("iterations", 0)

        # Format messages with the prompt
        prompt_messages = prompt.format_messages(messages=messages)

        # Call the model
        response = llm_with_tools.invoke(prompt_messages)

        logger.info(f"Agent iteration {iterations + 1}: Model response received")

        # Increment iteration counter
        return {"messages": [response], "iterations": iterations + 1}

    # Define the routing logic
    def should_continue(state: AgentState) -> Literal["tools", "end"]:
        """Determine whether to continue with tools or end."""
        messages = state["messages"]
        last_message = messages[-1]
        iterations = state.get("iterations", 0)

        # If max iterations reached, end
        if iterations >= max_iterations:
            logger.warning(f"Max iterations ({max_iterations}) reached, ending execution")
            return "end"

        # If the LLM makes a tool call, then we route to the "tools" node
        if hasattr(last_message, "tool_calls") and last_message.tool_calls:
            logger.info(f"Tool calls requested: {[tc['name'] for tc in last_message.tool_calls]}")
            return "tools"

        # Otherwise, we finish
        logger.info("No tool calls, ending execution")
        return "end"

    # Create the tool node
    tool_node = ToolNode(tools)

    # Build the graph
    workflow = StateGraph(AgentState)

    # Add nodes
    workflow.add_node("agent", call_model)
    workflow.add_node("tools", tool_node)

    # Set the entry point
    workflow.set_entry_point("agent")

    # Add conditional edges
    workflow.add_conditional_edges("agent", should_continue, {"tools": "tools", "end": END})

    # Add edge from tools back to agent
    workflow.add_edge("tools", "agent")

    # Compile the graph
    return workflow.compile()


def extract_thinking_steps(messages: List[BaseMessage]) -> List[dict]:
    """
    Extract thinking steps from the message history.

    Args:
        messages: List of messages from the agent execution

    Returns:
        List of thinking step dictionaries
    """
    thinking_steps = []

    # Iterate through messages and extract agent actions and tool results
    i = 0
    while i < len(messages):
        message = messages[i]

        # Check if this is an AI message with tool calls
        if isinstance(message, AIMessage) and hasattr(message, "tool_calls") and message.tool_calls:
            for tool_call in message.tool_calls:
                action_name = tool_call.get("name", "Unknown")
                action_input = tool_call.get("args", {})

                # Look for the corresponding tool message
                observation = ""
                for j in range(i + 1, len(messages)):
                    if isinstance(messages[j], ToolMessage) and messages[j].tool_call_id == tool_call.get("id"):
                        observation = messages[j].content
                        break

                thought = f"Calling tool: `{action_name}` with arguments: `{action_input}`"

                # Skip error/exception actions
                if action_name not in ["_Exception", "Invalid", "Error"] and "Invalid" not in observation:
                    thinking_steps.append(
                        {"thought": thought, "action": action_name, "action_input": str(action_input), "observation": observation}
                    )

        i += 1

    return thinking_steps


def get_final_response(messages: List[BaseMessage]) -> str:
    """
    Extract the final response from the agent.

    Args:
        messages: List of messages from the agent execution

    Returns:
        The final response text
    """
    # Get the last AI message without tool calls
    for message in reversed(messages):
        if isinstance(message, AIMessage):
            content = message.content
            
            # Handle when content is a list (e.g., Gemini with include_thoughts)
            if isinstance(content, list):
                # Extract text parts from the content list
                text_parts = []
                for part in content:
                    if isinstance(part, dict):
                        # Look for text in dict format
                        if "text" in part:
                            text_parts.append(part["text"])
                        elif "type" in part and part["type"] == "text" and "text" in part:
                            text_parts.append(part["text"])
                    elif isinstance(part, str):
                        text_parts.append(part)
                content = "\n".join(text_parts) if text_parts else ""
            
            # Only return if it's not a tool call message
            if not hasattr(message, "tool_calls") or not message.tool_calls:
                return content if content else "I apologize, but I couldn't generate a response."
            # If it has content alongside tool calls (streaming), return that
            elif content:
                return content

    return "I apologize, but I couldn't generate a response."
