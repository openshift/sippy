import fifaLogo from '../sippy-fifa.svg'
import logo from '../sippy.svg'

export function isWorldCup() {
  const now = new Date()
  const year = now.getFullYear()
  const month = now.getMonth()
  const day = now.getDate()
  return (
    year === 2026 && ((month === 5 && day >= 11) || (month === 6 && day <= 19))
  )
}

export function getSeasonalLogo() {
  return isWorldCup() ? fifaLogo : logo
}
