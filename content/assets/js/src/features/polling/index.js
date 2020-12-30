import { createEffect } from 'effector';
import { addStat, setStats } from '@features/stats';

const getStatsFx = createEffect('getStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/stats"
    const res = await fetch(url)
    const stats = await res.json()
    addStat(stats)
  }
})

const getCollectedStatsFx = createEffect('getCollectedStatsFx', {
  handler: async() => {
    const url = "/balansir/metrics/collected_stats"
    const res = await fetch(url)
    let result = await res.text()
    result = result.split('\n').map( JSON.parse ).flat();
    setStats(result)
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