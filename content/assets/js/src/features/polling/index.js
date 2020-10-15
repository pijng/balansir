import { createEffect } from 'effector';
import { addStat, setStats } from '../stats';

const getStatsFx = createEffect('getStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/stats"
    const res = await fetch(url)
    const stats = await res.json()
    addStat(stats)
  }
})

const getCollectedStatsFx = createEffect('getCollectedStatsFx', {
  handler: async(attempts = 1) => {
    if (attempts > 5) return

    const url = "/balansir/metrics/collected_stats"
    const res = await fetch(url)

    // Since collected logs file may be VERY large it's better to read it chunk by chunk,
    // than trying to parse the whole response at once
    const reader = res.body.getReader()
    const decoder = new TextDecoder()

    let result = ''
    /*eslint no-constant-condition: 0*/
    while(true) {
      const {done, value} = await reader.read()
      if (done) break

      result += decoder.decode(value)
    }

    try {
      setStats(JSON.parse(result))
    } catch {
      // The problem is that collected logs are returned as file by balansir handler,
      // instead of common 'application/json' response with body. Due to that we can
      // face the issue when file is requested by dashboard and being mutated by logger
      // at the same time. In that case the logs will have an invalid JSON format, so
      // `JSON.parse(result)` will throw an error. We re run the effect if that happens
      // to request collected logs again. This is a disgusting workaround, yet it does work
      // and helps us to avoid additional memory allocation on the server side to parse
      // collected logs.
      getCollectedStatsFx(attempts+1)
    }
  }
})

const getCollectedLogsFx = createEffect('getCollectedLogsFx', {
  handler: async() => {
    const url = "/balansir/logs/collected_logs"
    const res = await fetch(url)
    return res.json()
  }
})

export { getStatsFx, getCollectedStatsFx, getCollectedLogsFx };