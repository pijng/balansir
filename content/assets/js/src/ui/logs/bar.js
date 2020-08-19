import { h, spec } from 'forest';


const Bar = () => {
  h('div', () => {
    spec({ attr: {class: "logs-bar"} })

    h('div', {
      text: "Time",
      attr: {class: "time"}
    })

    h('div', {
      text: "Severity",
      attr: {class: "severity"}
    })

    h('div', {
      text: "Data",
      attr: {class: "data"}
    })
  })
}

export { Bar };