import { Box, Button, Paper, Typography } from '@mui/material'
import { ForumOutlined } from '@mui/icons-material'
import React from 'react'

export default function ChatTransition() {
  return (
    <Box
      sx={{
        alignItems: 'center',
        background:
          'radial-gradient(circle at top left, rgba(63, 81, 181, 0.18), transparent 45%), linear-gradient(135deg, #f6f8ff 0%, #ffffff 70%)',
        display: 'flex',
        justifyContent: 'center',
        minHeight: 'calc(100vh - 96px)',
        px: 3,
      }}
    >
      <Paper
        elevation={4}
        sx={{
          borderRadius: 4,
          maxWidth: 640,
          overflow: 'hidden',
          textAlign: 'center',
          width: '100%',
        }}
      >
        <Box sx={{ bgcolor: '#3f51b5', color: 'white', px: 4, py: 4 }}>
          <ForumOutlined sx={{ fontSize: 44, mb: 1 }} />
          <Typography component="h1" variant="h4">
            Sippy Chat has moved
          </Typography>
        </Box>
        <Box sx={{ px: 4, py: 5 }}>
          <Typography color="text.secondary" variant="h6">
            Sippy Chat is now part of Chai Bot.
          </Typography>
          <Typography color="text.secondary" sx={{ mt: 2 }}>
            Continue the conversation with Chai Bot in Slack.
          </Typography>
          <Button
            href="https://redhat.enterprise.slack.com/"
            rel="noreferrer"
            sx={{ mt: 4 }}
            target="_blank"
            variant="contained"
          >
            Open Slack
          </Button>
        </Box>
      </Paper>
    </Box>
  )
}
