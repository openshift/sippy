# React Integration Examples

This document shows how to integrate the Sippy Agent API with your React + Material UI frontend.

## API Endpoints

The FastAPI server provides these endpoints:

- `GET /health` - Health check
- `GET /status` - Agent status and available tools
- `POST /chat` - Send a message and get response (with optional thinking steps)
- `WebSocket /chat/stream` - Real-time streaming chat with thinking display

## React Service Example

```typescript
// services/sippyAgent.ts
export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp?: string;
}

export interface ThinkingStep {
  step_number: number;
  thought: string;
  action: string;
  action_input: string;
  observation: string;
}

export interface ChatResponse {
  response: string;
  thinking_steps?: ThinkingStep[];
  tools_used?: string[];
  error?: string;
}

export interface ChatRequest {
  message: string;
  chat_history?: ChatMessage[];
  show_thinking?: boolean;
}

class SippyAgentService {
  private baseUrl: string;
  private ws: WebSocket | null = null;

  constructor(baseUrl: string = 'http://localhost:8000') {
    this.baseUrl = baseUrl;
  }

  async sendMessage(request: ChatRequest): Promise<ChatResponse> {
    const response = await fetch(`${this.baseUrl}/chat`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    return response.json();
  }

  async getStatus() {
    const response = await fetch(`${this.baseUrl}/status`);
    return response.json();
  }

  connectWebSocket(
    onMessage: (data: any) => void,
    onError: (error: Event) => void
  ): void {
    const wsUrl = this.baseUrl.replace('http', 'ws') + '/chat/stream';
    this.ws = new WebSocket(wsUrl);

    this.ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      onMessage(data);
    };

    this.ws.onerror = onError;
  }

  sendWebSocketMessage(request: ChatRequest): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(request));
    }
  }

  disconnectWebSocket(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}

export default new SippyAgentService();
```

## React Component Example

```tsx
// components/SippyChat.tsx
import React, { useState, useEffect, useRef } from 'react';
import {
  Box,
  TextField,
  Button,
  Paper,
  Typography,
  List,
  ListItem,
  ListItemText,
  Chip,
  Switch,
  FormControlLabel,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  CircularProgress,
} from '@mui/material';
import { ExpandMore, Send, Psychology } from '@mui/icons-material';
import sippyAgent, { ChatMessage, ThinkingStep } from '../services/sippyAgent';

interface SippyChatProps {
  onStatusChange?: (status: string) => void;
}

export const SippyChat: React.FC<SippyChatProps> = ({ onStatusChange }) => {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [currentMessage, setCurrentMessage] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [showThinking, setShowThinking] = useState(false);
  const [currentThinking, setCurrentThinking] = useState<ThinkingStep[]>([]);
  const [useWebSocket, setUseWebSocket] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, currentThinking]);

  useEffect(() => {
    if (useWebSocket) {
      sippyAgent.connectWebSocket(
        (data) => {
          if (data.type === 'thinking_step') {
            setCurrentThinking(prev => {
              const newSteps = [...prev];
              const stepIndex = newSteps.findIndex(s => s.step_number === data.data.step_number);
              
              if (stepIndex >= 0) {
                // Update existing step
                newSteps[stepIndex] = { ...newSteps[stepIndex], ...data.data };
              } else {
                // Add new step
                newSteps.push(data.data);
              }
              
              return newSteps;
            });
          } else if (data.type === 'final_response') {
            setMessages(prev => [...prev, {
              role: 'assistant',
              content: data.data.response,
              timestamp: data.data.timestamp
            }]);
            setCurrentThinking([]);
            setIsLoading(false);
          } else if (data.type === 'error') {
            console.error('WebSocket error:', data.data.error);
            setIsLoading(false);
          }
        },
        (error) => {
          console.error('WebSocket error:', error);
          setIsLoading(false);
        }
      );

      return () => {
        sippyAgent.disconnectWebSocket();
      };
    }
  }, [useWebSocket]);

  const handleSendMessage = async () => {
    if (!currentMessage.trim()) return;

    const userMessage: ChatMessage = {
      role: 'user',
      content: currentMessage,
      timestamp: new Date().toISOString()
    };

    setMessages(prev => [...prev, userMessage]);
    setCurrentMessage('');
    setIsLoading(true);
    setCurrentThinking([]);

    const request = {
      message: currentMessage,
      chat_history: messages,
      show_thinking: showThinking
    };

    try {
      if (useWebSocket) {
        sippyAgent.sendWebSocketMessage(request);
      } else {
        const response = await sippyAgent.sendMessage(request);
        
        if (response.error) {
          throw new Error(response.error);
        }

        setMessages(prev => [...prev, {
          role: 'assistant',
          content: response.response,
          timestamp: new Date().toISOString()
        }]);

        if (response.thinking_steps) {
          setCurrentThinking(response.thinking_steps);
        }
      }
    } catch (error) {
      console.error('Error sending message:', error);
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: 'Sorry, I encountered an error processing your request.',
        timestamp: new Date().toISOString()
      }]);
    } finally {
      if (!useWebSocket) {
        setIsLoading(false);
      }
    }
  };

  const renderThinkingSteps = (steps: ThinkingStep[]) => (
    <Accordion>
      <AccordionSummary expandIcon={<ExpandMore />}>
        <Box display="flex" alignItems="center" gap={1}>
          <Psychology color="primary" />
          <Typography>Agent's Thinking Process ({steps.length} steps)</Typography>
        </Box>
      </AccordionSummary>
      <AccordionDetails>
        <List>
          {steps.map((step, index) => (
            <ListItem key={index} divider>
              <ListItemText
                primary={`Step ${step.step_number}`}
                secondary={
                  <Box>
                    {step.thought && (
                      <Typography variant="body2" color="text.secondary" paragraph>
                        <strong>Thought:</strong> {step.thought}
                      </Typography>
                    )}
                    {step.action && (
                      <Typography variant="body2" color="primary" paragraph>
                        <strong>Action:</strong> {step.action}
                      </Typography>
                    )}
                    {step.action_input && (
                      <Typography variant="body2" color="text.secondary" paragraph>
                        <strong>Input:</strong> {step.action_input}
                      </Typography>
                    )}
                    {step.observation && (
                      <Typography variant="body2" color="success.main">
                        <strong>Result:</strong> {step.observation}
                      </Typography>
                    )}
                  </Box>
                }
              />
            </ListItem>
          ))}
        </List>
      </AccordionDetails>
    </Accordion>
  );

  return (
    <Box display="flex" flexDirection="column" height="100%">
      {/* Controls */}
      <Box p={2} borderBottom={1} borderColor="divider">
        <Box display="flex" gap={2} alignItems="center">
          <FormControlLabel
            control={
              <Switch
                checked={showThinking}
                onChange={(e) => setShowThinking(e.target.checked)}
              />
            }
            label="Show Thinking"
          />
          <FormControlLabel
            control={
              <Switch
                checked={useWebSocket}
                onChange={(e) => setUseWebSocket(e.target.checked)}
              />
            }
            label="Real-time Streaming"
          />
        </Box>
      </Box>

      {/* Messages */}
      <Box flex={1} overflow="auto" p={2}>
        {messages.map((message, index) => (
          <Paper
            key={index}
            elevation={1}
            sx={{
              p: 2,
              mb: 2,
              backgroundColor: message.role === 'user' ? 'primary.light' : 'grey.100',
              color: message.role === 'user' ? 'primary.contrastText' : 'text.primary'
            }}
          >
            <Typography variant="body1">{message.content}</Typography>
          </Paper>
        ))}

        {/* Current thinking steps */}
        {currentThinking.length > 0 && renderThinkingSteps(currentThinking)}

        {/* Loading indicator */}
        {isLoading && (
          <Box display="flex" justifyContent="center" p={2}>
            <CircularProgress />
          </Box>
        )}

        <div ref={messagesEndRef} />
      </Box>

      {/* Input */}
      <Box p={2} borderTop={1} borderColor="divider">
        <Box display="flex" gap={1}>
          <TextField
            fullWidth
            variant="outlined"
            placeholder="Ask about CI jobs, test failures, or payloads..."
            value={currentMessage}
            onChange={(e) => setCurrentMessage(e.target.value)}
            onKeyPress={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSendMessage();
              }
            }}
            disabled={isLoading}
            multiline
            maxRows={4}
          />
          <Button
            variant="contained"
            onClick={handleSendMessage}
            disabled={isLoading || !currentMessage.trim()}
            startIcon={<Send />}
          >
            Send
          </Button>
        </Box>
      </Box>
    </Box>
  );
};
```

## Usage in Your App

```tsx
// In your main component or page
import { SippyChat } from './components/SippyChat';

function App() {
  return (
    <Container maxWidth="lg">
      <Typography variant="h4" gutterBottom>
        Sippy AI Agent
      </Typography>
      <Paper elevation={3} sx={{ height: '80vh' }}>
        <SippyChat />
      </Paper>
    </Container>
  );
}
```

## Starting the Server

```bash
# Install dependencies
pip install -r requirements.txt

# Start the web server
python web_main.py --host 0.0.0.0 --port 8000 --thinking

# Or with specific model
python web_main.py --model "gpt-4" --endpoint "https://api.openai.com/v1"
```

The server will be available at `http://localhost:8000` with API documentation at `http://localhost:8000/docs`.
