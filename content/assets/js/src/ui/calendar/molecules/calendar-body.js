import { h, list, remap, spec } from 'forest';
import { combine } from 'effector';
import { Range } from './range';
import { MonthLabel } from './month-label';
import { MonthBody } from './month-body';
import { Time } from './time';

const CalendarBody = (
  $uniqueDatesArray, $selectedSpan, selectSpan, daySelected, $spans,
  selectTime, $times, $monthSelected
  ) => {
  Range($selectedSpan, selectSpan)

  h('div', () => {
    spec({ attr: {class: "calendar-body"} })

    list($uniqueDatesArray, ({store}) => {
      const [year, months] = remap(store, ['year', 'months'])

      h('div', () => {
        spec({ attr: {class: "months"} })

        list(months, ({store, key: idx}) => {
          const month = remap(store, 'month')

          const $prevActive = combine(months, idx, (months, idx) => months.length-(months.length+idx) !== 0)
          const $nextActive = combine(months, idx, (months, idx) => months.length-1 !== idx)
          const $monthLength = months.map(months => months.length-1)
          const $selected = combine($monthSelected, $monthLength, (selected, length) => selected + length)
          const $invertedIdx = combine(idx, $monthLength, (idx, length) => idx - length)

          h('div', () => {
            spec({
              attr: {class: "month"},
              visible: combine($selected, idx, (selected, idx) => selected === idx)
            })

            MonthLabel($invertedIdx, year, month, $prevActive, $nextActive, $monthSelected)
            MonthBody(store, daySelected, year, month, $selectedSpan, $spans)
          })
        })

      })
    })

  })

  Time(selectTime, $times)
}

export { CalendarBody };