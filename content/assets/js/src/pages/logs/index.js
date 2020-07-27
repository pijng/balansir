import { h, spec } from 'forest';
import { Navigation, LogsBlock } from '../../ui';
import { $logs, openTagFilter, $isFilterVisible, focusSearch, $isSearchFocused, toggleTag, $tags, searchLogs, $searchInput, clearSearch } from '../../features/logs';

const Logs = () => {
  Navigation(document.location.pathname)

  h('div', () => {
    spec({ attr: {class: "container-view"} })

    h('div', () => {
      spec({ attr: {class: "logs-block"} })

      LogsBlock($logs, openTagFilter, $isFilterVisible, focusSearch, $isSearchFocused, toggleTag, $tags, searchLogs, $searchInput, clearSearch)
    })
  })
}

export { Logs };