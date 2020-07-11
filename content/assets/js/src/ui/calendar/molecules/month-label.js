import { h, spec } from 'forest';

const MONTH_PROJECTION = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"]

const MonthLabel = (year, month) => {
  h('div', () => {
    spec({ attr: {class: "month-label-wrapper"} })

    h('div', {
      attr: {class: "month-arrow month-back"},
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
      attr: {class: "month-arrow month-forward"},
      text: ">"
    })
  })
}

export { MonthLabel };