# Chat Components

## AskSippyButton

The `AskSippyButton` component provides a reusable button that opens the chat widget and pre-sends a question to Sippy.

### Basic Usage

```jsx
import AskSippyButton from '../chat/AskSippyButton'

function MyComponent() {
  return (
    <AskSippyButton
      question="Why is this test failing?"
      tooltip="Ask Sippy about this test"
    />
  )
}
```

### With Dynamic Content

```jsx
<AskSippyButton
  question={`Why is test "${testName}" failing in version ${version}?`}
  tooltip="Ask Sippy about this test failure"
  variant="contained"
  color="primary"
/>
```

### With Page Context

You can provide additional context to help Sippy answer questions more accurately:

```jsx
<AskSippyButton
  question="What are the common failures in this job?"
  context={{
    page: 'job-details',
    url: window.location.href,
    data: {
      jobName: jobName,
      jobId: jobId,
    },
  }}
  tooltip="Ask Sippy about job failures"
/>
```

### Props

| Prop        | Type    | Required | Default      | Description                                        |
| ----------- | ------- | -------- | ------------ | -------------------------------------------------- |
| `question`  | string  | Yes      | -            | The question to pre-send to Sippy                  |
| `context`   | object  | No       | undefined    | Page context to provide to the chat                |
| `tooltip`   | string  | No       | undefined    | Tooltip text to display on hover                   |
| `variant`   | string  | No       | 'outlined'   | Button variant: 'text', 'outlined', or 'contained' |
| `size`      | string  | No       | 'small'      | Button size: 'small', 'medium', or 'large'         |
| `color`     | string  | No       | 'primary'    | Button color theme                                 |
| `label`     | string  | No       | 'Ask Sippy'     | Button label text                                  |
| `startIcon` | node    | No       | AutoAwesomeIcon | Icon to display at the start of the button         |
| `disabled`  | boolean | No       | false        | Whether the button is disabled                     |
| `sx`        | object  | No       | undefined    | Additional Material-UI sx prop for custom styling  |

### Examples

#### Simple Question Button

```jsx
<AskSippyButton
  question="What is the current health of 4.15?"
  tooltip="Ask about release health"
/>
```

#### Custom Styling

```jsx
<AskSippyButton
  question="Show me the top 10 failing tests"
  variant="contained"
  color="secondary"
  size="large"
  label="Get Test Report"
  sx={{ marginTop: 2 }}
/>
```

#### Fancy Button with Gradient and Animation

```jsx
<AskSippyButton
  question="Why is this test failing?"
  tooltip="Ask Sippy AI about this test"
  variant="contained"
  size="medium"
  sx={{
    background: 'linear-gradient(45deg, #2196F3 30%, #21CBF3 90%)',
    boxShadow: '0 3px 5px 2px rgba(33, 203, 243, .3)',
    color: 'white',
    fontWeight: 'bold',
    textTransform: 'none',
    transition: 'all 0.3s ease',
    animation: 'pulse 2s ease-in-out infinite',
    '@keyframes pulse': {
      '0%, 100%': {
        boxShadow: '0 3px 5px 2px rgba(33, 203, 243, .3)',
      },
      '50%': {
        boxShadow: '0 3px 15px 5px rgba(33, 203, 243, .5)',
      },
    },
    '&:hover': {
      background: 'linear-gradient(45deg, #1976D2 30%, #00BCD4 90%)',
      boxShadow: '0 6px 20px 4px rgba(33, 203, 243, .4)',
      transform: 'translateY(-2px)',
    },
  }}
/>
```

#### Icon-only Button

```jsx
<AskSippyButton
  question="Help me debug this issue"
  label=""
  tooltip="Ask Sippy for help"
  variant="text"
  size="small"
/>
```

### Direct API Usage

If you need more control, you can use the `askQuestion` function directly:

```jsx
import { useGlobalChat } from '../chat/useGlobalChat'

function MyComponent() {
  const { askQuestion } = useGlobalChat()

  const handleClick = () => {
    askQuestion('Why is this test failing?', {
      page: 'my-page',
      data: { someData: 'value' },
    })
  }

  return <Button onClick={handleClick}>Custom Button</Button>
}
```

## Implementation Details

The `askQuestion` function:

1. Updates the page context if provided
2. Opens the chat widget
3. Automatically sends the question after a brief delay (100ms) to ensure the chat is rendered

The chat widget will display with the question already sent, and Sippy will begin processing it immediately.
