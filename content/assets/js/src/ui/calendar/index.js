import { h, spec } from 'forest';
import { CalendarBody } from './molecules/calendar-body';

const Calendar = ($uniqueDatesArray, $selectedSpan, selectSpan, $isVisible,
  daySelected, $spans, selectTime, $times, $monthSelected
  ) => {
  h('div', () => {
    spec({ attr: {class: "date-picker"} })

    h('div', () => {
      spec({ attr: {class: "filter"} })

      h('div', {
        attr: {class: "filter-label"},
        text: "Filted date",
      })
    })

    h('div', () => {
      spec({
        attr: {class: "calendar"},
        visible: $isVisible,
      })

      CalendarBody($uniqueDatesArray, $selectedSpan, selectSpan, daySelected,
        $spans, selectTime, $times, $monthSelected
      )
    })

  })
}

export { Calendar };