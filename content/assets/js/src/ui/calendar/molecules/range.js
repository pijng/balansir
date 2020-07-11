import { h, spec, remap } from 'forest';

const Range = ($selectedSpan, selectSpan) => {
  h('div', () => {
    spec({ attr: {class: "dates"} })

    const [from, to] = remap($selectedSpan, ['from', 'to'])

    h('div', {
      attr: {class: from.map(v => v ? "date-box from active" : "date-box from")},
      text: "From",
      handler: { click: selectSpan.prepend(() => ({from: true, to: false})) }
    })
    h('div', {
      attr: {class: to.map(v => v ? "date-box to active" : "date-box to")},
      text: "To",
      handler: { click: selectSpan.prepend(() => ({from: false, to: true})) }
    })
  })
}

export { Range };