import { createEffect } from 'effector';

const getStatsFx = createEffect('getStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/stats"
    const req = await fetch(url)
    return req.json()
  }
})

const getCollectedStatsFx = createEffect('getCollectedStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/collected_stats"
    const req = await fetch(url)
    return req.json()
  }
})

export { getStatsFx, getCollectedStatsFx };