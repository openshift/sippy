import { format, utcToZonedTime } from 'date-fns-tz'
import DateFnsUtils from '@date-io/date-fns'
export class GridToolbarFilterDateUtils extends DateFnsUtils {
  constructor() {
    super()
  }

  format(date, formatString) {
    return `${format(utcToZonedTime(date, 'UTC'), formatString, {
      timeZone: 'Etc/UTC',
      locale: this.locale,
    })}`
  }
}
