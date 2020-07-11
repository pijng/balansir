import { createEffect } from 'effector';

export const getStatsFx = createEffect({
  handler: async() => {
    const url = "/balansir/metrics/stats"
    const req = await fetch(url)
    return req.json()
  }
})