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

  // This changes the text header in the DateTimePicker.
  // The captial HH will make it 24 hour format.
  getHourText(date, ampm) {
    return format(utcToZonedTime(date, 'UTC'), 'HH')
  }
}
