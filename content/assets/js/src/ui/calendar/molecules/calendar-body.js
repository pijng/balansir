import { h, list, remap, spec } from 'forest';
import { combine, sample, createStore, guard } from 'effector';
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

        const $relevantMonthSelected = createStore(false)
        const initRelevantMonth = sample({
          source: months,
          clock: guard({
            source: $relevantMonthSelected,
            filter: (selected) => !selected
          }),
          fn: (months) => months.length - 1,
          target: $monthSelected
        })
        $relevantMonthSelected.on(initRelevantMonth, () => true)

        list(months, ({store, key: idx}) => {
          const month = remap(store, 'month')

          const $prevActive = combine(months, idx, (months, idx) => months.length-(months.length+idx) !== 0)
          const $nextActive = combine(months, idx, (months, idx) => months.length-1 !== idx)
  
          h('div', () => {
            spec({
              attr: {class: "month"},
              visible: combine($monthSelected, idx, (selected, idx) => selected === idx)
            })

            MonthLabel(idx, year, month, $prevActive, $nextActive, $monthSelected)
            MonthBody(store, daySelected, year, month, $selectedSpan, $spans)
          })
        })

      })
    })

  })

  Time(selectTime, $times)
}

export { CalendarBody };