import { h, spec } from 'forest';

const MajorLabel = ($value, cls, label) => {
  h('div', () => {
    spec({ attr: {class: `chart-label ${cls}`} })

    h('p', { text: $value })
    h('p', { text: $value > 1000 ? "s" : "ms" })
    h('span', { text: label })
  })
}

export { MajorLabel };