import { using, h, spec, variant, node } from 'forest';
import { getStatsFx, getCollectedStatsFx } from './features/polling';
import { Metrics } from './pages/metrics';
import { openCalendar } from './features/calendar';
import { createStore } from 'effector';
import { Logs } from './pages/logs';

using(document.querySelector('body'), () => {
  const location = createStore(window.location.pathname)

  variant({
    source: location,
    key: path => path,
    cases: {

      "/balansir/metrics": () => {
        h('section', () => {
          spec({
            attr: {class: "layout"},
            handler: {click: openCalendar.prepend(e => e)}
          })
          Metrics()
          node(() => {
            setTimeout(getCollectedStatsFx)
            setInterval(getStatsFx, 1000)
          })
        })
      },

      "/balansir/logs": () => {
        h('section', () => {
          spec({
            attr: {class: "layout"}
          })
          Logs()
        })
      }

    }
  })
})