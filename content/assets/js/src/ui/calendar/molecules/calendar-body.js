import { h, list, remap, spec } from 'forest';
import { Range } from './range';
import { MonthLabel } from './month-label';
import { MonthBody } from './month-body';
import { Time } from './time';

const CalendarBody = (
  $uniqueDatesArray, $selectedSpan, selectSpan, daySelected, $spans, 
  selectTime, $times
  ) => {
  Range($selectedSpan, selectSpan)

  h('div', () => {
    spec({ attr: {class: "calendar-body"} })

    list($uniqueDatesArray, ({store}) => {
      const [year, months] = remap(store, ['year', 'months'])

      h('div', () => {
        spec({ attr: {class: "months"} })

        list(months, ({store}) => {
          const month = remap(store, 'month')
  
          h('div', () => {
            spec({ attr: {class: "month"} })

            MonthLabel(year, month)
            MonthBody(store, daySelected, year, month, $selectedSpan, $spans)
          })
        })

      })
    })

  })

  Time(selectTime, $times)
}

export { CalendarBody };