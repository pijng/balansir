import { createEffect } from 'effector';

const getStatsFx = createEffect('getStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/stats"
    const res = await fetch(url)
    return res.json()
  }
})

const getCollectedStatsFx = createEffect('getCollectedStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/collected_stats"
    const res = await fetch(url)
    const stats = await res.json()
    return stats
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