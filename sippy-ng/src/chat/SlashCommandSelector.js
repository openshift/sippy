import {
  Chip,
  ClickAwayListener,
  List,
  ListItem,
  ListItemText,
  Paper,
  Popper,
} from '@mui/material'
import { Computer as ComputerIcon } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import { usePrompts } from './store/useChatStore'
import PropTypes from 'prop-types'
import React, { useEffect, useRef, useState } from 'react'

const useStyles = makeStyles((theme) => ({
  popper: {
    zIndex: theme.zIndex.modal + 1,
  },
  list: {
    maxHeight: 400,
    overflow: 'auto',
  },
  listItem: {
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  listItemContent: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    width: '100%',
  },
  localChip: {
    marginLeft: 'auto',
  },
}))

export default function SlashCommandSelector({
  anchorEl,
  filterText = '',
  open,
  onSelect,
  onClose,
  onNavigate,
  placement = 'top-start',
}) {
  const classes = useStyles()
  const { prompts } = usePrompts()
  const [selectedIndex, setSelectedIndex] = useState(0)
  const selectedItemRef = useRef(null)

  // Filter and sort visible prompts
  const visiblePrompts = prompts
    .filter((prompt) => !prompt.hide)
    .sort((a, b) => a.name.localeCompare(b.name))

  const filteredPrompts = visiblePrompts.filter((prompt) =>
    prompt.name.toLowerCase().includes(filterText.toLowerCase())
  )

  // Reset selected index when filtered prompts change
  useEffect(() => {
    setSelectedIndex(0)
  }, [filteredPrompts.length, filterText])

  // Scroll selected item into view
  useEffect(() => {
    if (selectedItemRef.current) {
      selectedItemRef.current.scrollIntoView({
        block: 'nearest',
        behavior: 'smooth',
      })
    }
  }, [selectedIndex])

  // Expose navigation methods to parent via callback
  useEffect(() => {
    if (onNavigate && filteredPrompts.length > 0) {
      onNavigate({
        moveNext: () =>
          setSelectedIndex((prev) =>
            prev >= filteredPrompts.length - 1 ? 0 : prev + 1
          ),
        movePrevious: () =>
          setSelectedIndex((prev) =>
            prev <= 0 ? filteredPrompts.length - 1 : prev - 1
          ),
        selectCurrent: () => {
          if (filteredPrompts[selectedIndex]) {
            onSelect(filteredPrompts[selectedIndex])
          }
        },
      })
    }
  }, [onNavigate, filteredPrompts, selectedIndex, onSelect])

  const handlePromptClick = (prompt) => {
    onSelect(prompt)
    if (onClose) {
      onClose()
    }
  }

  const handleClickAway = () => {
    if (onClose) {
      onClose()
    }
  }

  const shouldShow = open && filteredPrompts.length > 0

  if (!shouldShow) {
    return null
  }

  const content = (
    <Paper elevation={4}>
      <List className={classes.list}>
        {filteredPrompts.map((prompt, index) => (
          <ListItem
            key={prompt.name}
            ref={index === selectedIndex ? selectedItemRef : null}
            className={classes.listItem}
            onClick={() => handlePromptClick(prompt)}
            selected={index === selectedIndex}
          >
            <div className={classes.listItemContent}>
              <ListItemText
                primary={`/${prompt.name}`}
                secondary={prompt.description}
              />
              {prompt.source === 'local' && (
                <Chip
                  icon={<ComputerIcon />}
                  label="Local"
                  size="small"
                  color="secondary"
                  variant="outlined"
                  className={classes.localChip}
                />
              )}
            </div>
          </ListItem>
        ))}
      </List>
    </Paper>
  )

  return (
    <Popper
      open={shouldShow}
      anchorEl={anchorEl}
      placement={placement}
      className={classes.popper}
    >
      {onClose ? (
        <ClickAwayListener onClickAway={handleClickAway}>
          {content}
        </ClickAwayListener>
      ) : (
        content
      )}
    </Popper>
  )
}

SlashCommandSelector.propTypes = {
  anchorEl: PropTypes.object,
  filterText: PropTypes.string,
  open: PropTypes.bool.isRequired,
  onSelect: PropTypes.func.isRequired,
  onClose: PropTypes.func,
  onNavigate: PropTypes.func,
  placement: PropTypes.string,
}
