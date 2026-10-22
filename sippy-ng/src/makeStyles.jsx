import { css } from '@emotion/css'
import { useTheme } from '@mui/material/styles'
import React from 'react'

export function makeStyles(stylesOrCreator) {
  let cache = null
  let cacheKey = null

  return function useStyles(props) {
    const theme = useTheme()

    if (cache && cacheKey === theme) {
      if (!hasDynamicStyles(stylesOrCreator, theme)) {
        return cache
      }
    }

    const resolved =
      typeof stylesOrCreator === 'function'
        ? stylesOrCreator(theme)
        : stylesOrCreator

    const classes = {}
    for (const key of Object.keys(resolved)) {
      const value = resolved[key]
      classes[key] =
        typeof value === 'function' ? css(value(props)) : css(value)
    }

    if (!hasDynamicStyles(stylesOrCreator, theme)) {
      cache = classes
      cacheKey = theme
    }

    return classes
  }
}

function hasDynamicStyles(stylesOrCreator, theme) {
  const resolved =
    typeof stylesOrCreator === 'function'
      ? stylesOrCreator(theme)
      : stylesOrCreator
  return Object.values(resolved).some((v) => typeof v === 'function')
}

export function withStyles(stylesOrCreator) {
  return function wrap(Component) {
    const Wrapped = React.forwardRef(function StyledComponent(props, ref) {
      const theme = useTheme()
      const resolved =
        typeof stylesOrCreator === 'function'
          ? stylesOrCreator(theme)
          : stylesOrCreator

      const classes = {}
      for (const key of Object.keys(resolved)) {
        classes[key] = css(resolved[key])
      }

      return <Component {...props} classes={classes} ref={ref} />
    })

    Wrapped.displayName = `WithStyles(${Component.displayName || Component.name || 'Component'})`
    return Wrapped
  }
}
