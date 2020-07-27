import { h, spec } from 'forest';

const Navigation = (location) => {
  h('div', () => {
    spec({ attr: {class: "navigation"} })

    h('div', () => {
      spec({ attr: {class: "brand"} })

      h('img', { attr: {src: "/content/assets/img/balansir.png"} })
    })

    h('div', () => {
      spec({ attr: {class: "menu"} })

      h('a', () => {
        spec({
          attr: {
            class: `item ${location == '/balansir/metrics' ? 'active' : ''}`,
            href: '/balansir/metrics'
          }
        })
        
        h('span', { text: "Metrics" })
      })

      h('a', () => {
        spec({
          attr: {
            class: `item ${location == '/balansir/servers' ? 'active' : ''}`,
            href: '/balansir/servers'
          }
        })
        
        h('span', { text: "Servers" })
      })

      h('a', () => {
        spec({
          attr: {
            class: `item ${location == '/balansir/logs' ? 'active' : ''}`,
            href: '/balansir/logs'
          }
        })
        
        h('span', { text: "Logs" })
      })

      h('a', () => {
        spec({
          attr: {
            class: `item ${location == '/balansir/cache' ? 'active' : ''}`,
            href: '/balansir/cache'
          }
        })
        
        h('span', { text: "Cache" })
      })

      h('a', () => {
        spec({
          attr: {
            class: `item ${location == '/balansir/configuration' ? 'active' : ''}`,
            href: '/balansir/configuration'
          }
        })
        
        h('span', { text: "Configuration" })
      })

    })

  })
}

export { Navigation };