import { h, spec } from 'forest';

const MinorChart = ($value, label, chartId) => {
  h('div', () => {
    spec({ attr: {class: "chart-wrapper chart-small"} })

    h('div', () => {
      spec({ attr: {class: "chart-label"} })

      h('p', { text: $value })
      
      h('span', { text: `${label}` })
    })

    h('div', () => {
      spec({ attr: {class: "chart-wrapper_area"} })

      h('canvas', { attr: {id: `${chartId}`} })
    })

  })
}

export { MinorChart };