import { createStore, forward, createEvent, guard, sample } from 'effector';
import { getCollectedLogsFx } from '../polling';

const $logs = createStore([])

forward({
  from: getCollectedLogsFx.doneData,
  to: $logs
})

const openTagFilter = createEvent()
const $isFilterVisible = createStore(false)
  .on(openTagFilter, (state, params) => {
    if (params.target.className === 'filter' || params.target.className === 'filter-label') {
      return !state
    }
    if (params.composedPath().map(p => p.className).includes('tags-filter')) {
      return state
    }
    return false
  })

const focusSearch = createEvent()
const $isSearchFocused = createStore(false)
  .on(focusSearch, (_, params) => params.target.className === 'input')

const terminateActiveEntities = createEvent()
const guardedTerminate = guard({
  source: terminateActiveEntities,
  filter: (params) => {
    return !(params.target.className === 'filter' || params.target.className === 'filter-label')
  }
})

forward({
  from: guardedTerminate,
  to: [openTagFilter, focusSearch]
})

const $tags = createStore([
  {name: "INFO", checked: true},
  {name: "NOTICE", checked: true},
  {name: "WARNING", checked: true},
  {name: "ERROR", checked: true},
  {name: "FATAL", checked: true}
])

const toggleTag = createEvent()

sample({
  source: $tags,
  clock: toggleTag,
  fn: (tags, name) => {
    return tags.map(tag => {
      if (tag.name === name) {
        return {...tag, checked: !tag.checked}
      }
      return tag
    })
  },
  target: $tags
})

const searchLogs = createEvent()
const clearSearch = createEvent()
const $searchInput = createStore("")
  .on(searchLogs, (_, params) => params)
  .reset(clearSearch)

export {
  $logs,
  openTagFilter,
  $isFilterVisible,
  focusSearch,
  $isSearchFocused,
  terminateActiveEntities,
  toggleTag,
  $tags,
  searchLogs,
  $searchInput,
  clearSearch
};