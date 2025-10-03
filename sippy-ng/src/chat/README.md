# Global Chat Widget

The global chat widget is available on every page in Sippy and provides contextual AI assistance.

## Features

- **Global Access**: Available on every page via a floating action button (FAB)
- **Contextual Awareness**: Automatically knows what page you're on and can access page-specific data
- **Streaming Responses**: Real-time streaming of AI responses with thinking steps
- **Personas**: Support for different AI personas (if configured)
- **WebSocket Connection**: Low-latency bidirectional communication

## Components

### GlobalChatWidget
The main chat drawer that slides out from the right side of the screen.

**Props:**
- `open` (boolean): Controls whether the drawer is visible
- `onClose` (function): Callback when the drawer should close
- `pageContext` (object): Context information about the current page

### FloatingChatButton
The floating action button that opens the chat widget.

**Props:**
- `onClick` (function): Callback when the button is clicked
- `unreadCount` (number): Number of unread messages (optional)
- `hasContext` (boolean): Whether page context is available
- `disabled` (boolean): Whether the button is disabled

### useGlobalChat Hook
React hook for managing global chat state.

**Provides:**
- `isOpen` (boolean): Whether the chat is currently open
- `pageContext` (object): Current page context
- `unreadCount` (number): Number of unread messages
- `openChat(context)` (function): Opens the chat with optional context
- `closeChat()` (function): Closes the chat
- `toggleChat(context)` (function): Toggles the chat open/closed
- `updatePageContext(context)` (function): Updates the current page context
- `incrementUnreadCount()` (function): Increments the unread message counter

## Adding Page Context

To provide context from your page to the chat assistant, use the `useGlobalChat` hook:

```javascript
import { useGlobalChat } from '../chat/useGlobalChat'

function MyPageComponent() {
  const { updatePageContext } = useGlobalChat()
  
  // Update context when relevant data changes
  useEffect(() => {
    updatePageContext({
      page: 'payload-details',
      url: window.location.href,
      data: {
        release: release,
        payload: payloadTag,
        blockingJobs: failedJobs.map(j => ({
          name: j.name,
          id: j.id,
          url: j.url
        })),
        state: payloadState
      }
    })
  }, [release, payloadTag, failedJobs, payloadState, updatePageContext])
  
  return (
    // ... your page content
  )
}
```

## Page Context Structure

The page context should follow this structure:

```javascript
{
  page: string,        // Page identifier (e.g., 'payload-details', 'job-analysis')
  url: string,         // Current page URL
  data: {              // Page-specific data
    // Any relevant data the AI might need to help the user
    // Keep it focused on what's currently visible/relevant
  }
}
```

### Example Contexts

**Payload Details Page:**
```javascript
{
  page: 'payload-details',
  url: '/sippy-ng/release/4.16/payloads/4.16.0-0.nightly-2025-10-03-123456',
  data: {
    release: '4.16',
    payload: '4.16.0-0.nightly-2025-10-03-123456',
    state: 'Rejected',
    blockingJobs: [
      { name: 'periodic-ci-openshift-release-master-ci-4.16...', id: '1234', url: '...' }
    ]
  }
}
```

**Job Analysis Page:**
```javascript
{
  page: 'job-analysis',
  url: '/sippy-ng/jobs/analysis/1234',
  data: {
    jobName: 'periodic-ci-openshift-release-master-ci-4.16-e2e-gcp',
    jobId: '1234',
    jobUrl: 'https://prow.ci.openshift.org/view/gs/...',
    failedTests: ['test1', 'test2', 'test3']
  }
}
```

**Release Overview Page:**
```javascript
{
  page: 'release-overview',
  url: '/sippy-ng/release/4.16',
  data: {
    release: '4.16',
    currentHealth: 85.5,
    recentPayloads: [...],
    majorFailures: [...]
  }
}
```

## Best Practices

1. **Keep Context Minimal**: Only include data that's currently visible or relevant to the user
2. **Update on Change**: Update the context when significant page state changes
3. **Clear IDs**: Include unique identifiers (job IDs, payload tags, etc.) for the AI to use with tools
4. **User-Facing Names**: Include user-friendly display names alongside IDs
5. **URLs**: Include relevant URLs for easy reference

## Backend Integration

The page context is sent to the backend with each message:

```python
# In web_server.py
data = await websocket.receive_text()
request_data = json.loads(data)

message = request_data.get("message", "")
page_context = request_data.get("page_context", {})  # Page context from frontend
```

The agent can then use this context to provide more relevant assistance without requiring the user to provide details manually.


