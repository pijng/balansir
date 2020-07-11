import { sample, createStore, createEvent } from 'effector';
import { $stats } from '../stats';
import { selectRange } from '../segmented_control';

const $uniqueDates = sample({
  source: $stats,
  fn: stats => stats.reduce((acc, elem) => {
    const [year, month, day]  = [new Date(elem.timestamp).getFullYear(), new Date(elem.timestamp).getMonth(), new Date(elem.timestamp).getDate()]
    acc[year] = acc[year] || {}
    acc[year][month] = [ ...new Set( [...(acc[year][month] || []), day] ) ]
    return acc;
  },{})
})

const $uniqueDatesArray = sample({
  source: $uniqueDates,
  fn: (state) => {
    let arr = []
    const keys = Object.keys(state)
    for (const k of keys) {
      const year = arr.find(a => a.year === k) || {year: k, months:[]}
      for (const month in state[k]) {
        const daysInMonth = new Date(year.year, parseInt(month)+1, 0).getDate()
        const startingDay = new Date(year.year, parseInt(month), 1).getDay()-1
        year.months = [
          ...year.months,
          {
            month: month,
            activeDays: state[k][month],
            numberOfDays: [
              ...Array.from(Array(startingDay), () => 0),
              ...Array.from(Array(daysInMonth), (_,d)=>d+1)
            ]
          }
        ]
      }
      arr = [...arr, year]
    }
    return arr
  }
})


const $spans = createStore({
  from: {active: false, date: null, time: ""},
  to: {active: false, date: null, time: ""}
})
$spans.reset(selectRange)

const selectSpan = createEvent()
const $selectedSpan = createStore({from: true, to: false})
  .on(selectSpan, (_, params) => params)

const openCalendar = createEvent()
const $isVisible = createStore(false)
  .on(openCalendar, (state, params) => {
    if (params.target.className === 'filter' || params.target.className === 'filter-label') {
      return !state
    }
    if (params.path.map(p => p.className).includes("calendar")) {
      return state
    }
    return false
  })


const selectTime = createEvent()
const $times = createStore("")
$times.on(selectTime, (_, params) => params)
$times.reset(selectRange)

sample({
  source: $spans,
  clock: selectSpan,
  fn: (spans, selected) => {
    if (selected.from) {
      return spans.from.time
    }
    return spans.to.time
  },
  target: $times
})

sample({
  source: [$spans, $selectedSpan],
  clock: $times,
  fn: ([spans, selected], times) => {
    var hour, minutes
    if (times) {
      [hour, minutes] = times.split(':')
    } else {
      [hour, minutes] = ["", ""]
    }

    if (selected.from) {
      const date = new Date(spans.from.date)
      date.setHours(hour)
      date.setMinutes(minutes)
      return {
        from: {active: spans.from.active, date: date.getTime(), time: times},
        to: {...spans.to}
      }
    }
    if (selected.to) {
      const date = new Date(spans.to.date)
      date.setHours(hour)
      date.setMinutes(minutes)
      return {
        from: {...spans.from},
        to: {active: spans.to.active, date: date.getTime(), time: times}
      }
    }
  },
  target: $spans
})

export {
  $uniqueDatesArray, $selectedSpan, selectSpan, openCalendar,
  $isVisible, selectTime, $times, $spans
};