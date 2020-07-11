import { h, spec } from 'forest';

const Navigation = () => {
  h('div', () => {
    spec({ attr: {class: "navigation"} })

    h('div', () => {
      spec({ attr: {class: "brand"} })

      h('img', { attr: {src: "/content/assets/img/balansir.png"} })
    })

    h('div', () => {
      spec({ attr: {class: "menu"} })

      h('div', () => {
        spec({ attr: {class: "item active"} })
        
        h('span', { text: "Metrics" })
      })

      h('div', () => {
        spec({ attr: {class: "item"} })
        
        h('span', { text: "Servers" })
      })

      h('div', () => {
        spec({ attr: {class: "item"} })
        
        h('span', { text: "Cache" })
      })

      h('div', () => {
        spec({ attr: {class: "item"} })
        
        h('span', { text: "Configuration" })
      })

    })

  })
}

export { Navigation };