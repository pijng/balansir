import { h, spec, list, variant, remap } from 'forest';
import { createStore, createEvent, sample, combine, guard } from 'effector';

const WEEK_DAYS = createStore(["mon", "tue", "wed", "thu", "fri", "sat", "sun"])

const MonthBody = ($store, daySelected, year, month, $selectedSpan, $spans) => {
  h('div', () => {
    spec({ attr: {class: "daysheader"} })

    list(WEEK_DAYS, ({store}) => {
      h('div', {
        attr: {class: "day"},
        text: store,
      })
    })
  })

  h('div', () => {
    spec({ attr: {class: "days"} })

    const [$activeDays, $numberOfDays] = remap($store, ['activeDays', 'numberOfDays'])

    list($numberOfDays, ({store}) => {
      variant({
        source: store,
        key: day => day === 0 ? "blank" : "filled",
        cases: {
          blank: () => {
            h('div', {
              attr: {class: "cell"},
              text: ""
            })
          },
          filled: ({store}) => {
            const selectDay = createEvent()
            guard({
              source: sample({
                source: [$activeDays, store, month, year],
                clock: selectDay,
                fn: ([activeDays, day, month, year]) => ({activeDays, day, month, year})
              }),
              filter: ({activeDays, day}) => activeDays.includes(day),
              target: daySelected
            })

            const $class = combine({
              act: $activeDays,
              day: store,
              month: month,
              year: year,
              sel: $selectedSpan,
              spn: $spans
              },({act, day, month, year, sel, spn}) => {
                if (act.includes(day)) {
                  const thisDate = new Date(year, month, day).toDateString()
                  if (sel.from && spn.from.active) {
                    const selectedDate = new Date(spn.from.date).toDateString()
                    if (thisDate === selectedDate) return 'active cell selected'
                  }
                  if (sel.to && spn.to.active) {
                    const selectedDate = new Date(spn.to.date).toDateString()
                    if (thisDate === selectedDate) return 'active cell selected'
                  }
                  return 'active cell'
                }
                return 'cell'
              }
            )
            
            h('div', {
              attr: {class: $class},
              text: store,
              handler: {click: selectDay}
            })
          }
        }
      })
    })
  })
}

export { MonthBody };