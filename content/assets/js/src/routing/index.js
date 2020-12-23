import { h, spec, variant, node } from 'forest';
import { createStore } from 'effector';
import { getCollectedStatsFx, getStatsFx, getCollectedLogsFx } from '@features/polling';
import { openCalendar } from '@features/calendar';
import { Metrics } from '@pages/metrics';
import { Logs } from '@pages/logs';
import { terminateActiveEntities } from '@features/logs';

const RoutePages = () => {
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
            attr: {class: "layout"},
            handler: {click: terminateActiveEntities.prepend(e => e)}
          })
          Logs()

          node(() => {
            setTimeout(getCollectedLogsFx)
            setInterval(getCollectedLogsFx, 1000)
          })
        })
      }

    }
  })
}

export { RoutePages };