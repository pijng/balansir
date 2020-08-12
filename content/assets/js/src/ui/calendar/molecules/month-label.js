import { h, spec } from 'forest';
import { createEvent, sample, guard } from 'effector';

const MONTH_PROJECTION = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"]

const MonthLabel = (idx, year, month, $prevActive, $nextActive, $monthSelected) => {
  const selectPrevMonth = createEvent()
  const selectNextMonth = createEvent()

  const guardedSelectedPrevMonth = guard({
    source: sample($prevActive, selectPrevMonth),
    filter: (prevActive) => prevActive
  })

  sample({
    source: idx,
    clock: guardedSelectedPrevMonth,
    fn: (idx) => idx - 1,
    target: $monthSelected
  })

  const guardedSelectedNextMonth = guard({
    source: sample($nextActive, selectNextMonth),
    filter: (nextActive) => nextActive
  })

  sample({
    source: idx,
    clock: guardedSelectedNextMonth,
    fn: (idx) => idx + 1,
    target: $monthSelected
  })

  h('div', () => {
    spec({ attr: {class: "month-label-wrapper"} })

    h('div', {
      attr: {class: $prevActive.map(
        active => active ? "month-arrow month-back active" : "month-arrow month-back"
      )},
      handler: {click: selectPrevMonth},
      text: "<"
    })

    h('div', {
      attr: {class: "month-name"},
      text: month.map(m => MONTH_PROJECTION[m])
    })
    h('div', {
      attr: {class: "year-name"},
      text: year
    })

    h('div', {
      attr: {class: $nextActive.map(
        active => active ? "month-arrow month-forward active" : "month-arrow month-forward"
      )},
      handler: {click: selectNextMonth},
      text: ">"
    })
  })
}

export { MonthLabel };