import { grey } from '@mui/material/colors'
import React from 'react'

const drawerWidth = 240

export function buildCRStyles(theme) {
  return {
    menuButton: { marginRight: theme.spacing(2) },
    hide: { display: 'none' },
    drawer: { width: drawerWidth, flexShrink: 0 },
    drawerPaper: { width: drawerWidth },
    drawerHeader: {
      display: 'flex',
      alignItems: 'center',
      padding: theme.spacing(0, 1),
      ...theme.mixins.toolbar,
      justifyContent: 'flex-end',
    },
    selectedJobRun: {
      borderStyle: 'solid',
      borderWidth: '1px',
      borderRadius: '5px',
      marginRight: '1px',
    },
    unselectedJobRun: { marginRight: '1px' },
    crColResultFull: {
      backgroundColor: theme.palette.mode === 'dark' ? grey[800] : 'whitesmoke',
      fontWeight: 'bold',
      position: 'sticky',
      top: 0,
      left: 0,
      zIndex: 1,
    },
    crColResult: {
      hyphens: 'auto',
      verticalAlign: 'top !important',
      backgroundColor: theme.palette.mode === 'dark' ? grey[800] : 'whitesmoke',
      fontWeight: 'bold',
      position: 'sticky',
      top: 0,
      left: 0,
      zIndex: 1,
      wordBreak: 'break-word',
    },
    crColJobName: {
      verticalAlign: 'top',
      backgroundColor: theme.palette.mode === 'dark' ? grey[900] : grey['A200'],
    },
    componentName: {
      width: 220,
      minWidth: 220,
      maxWidth: 220,
      backgroundColor: theme.palette.mode === 'dark' ? grey[800] : 'whitesmoke',
      fontWeight: 'bold',
      position: 'sticky',
      left: 0,
      zIndex: 1,
    },
    crCellResult: {
      backgroundColor: theme.palette.mode === 'dark' ? grey[100] : 'white',
      minHeight: 60,
      padding: '8px !important',
      lineHeight: '16px !important',
      border: '1px solid #EEE',
      textAlign: 'center',
    },
    crCellName: { fontSize: '14px !important' },
    crCellCapabCol: { fontSize: '13px !important', width: '300px' },
  }
}

export const ComponentReadinessStyleContext = React.createContext({})
