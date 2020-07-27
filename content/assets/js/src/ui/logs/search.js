import { h, spec } from 'forest';

const Search = (focusSearch, $isSearchFocused, searchLogs, clearSearch, $searchInput) => {
  h('div', () => {
    spec({
      attr: {class: $isSearchFocused.map(v => v ? "search focused" : "search")}
    })
    
    h('img', {
      attr: {
        class: "search-icon",
        src: "/content/assets/img/icons/search.svg"
      }
    })
  
    h('input', {
      attr: {
        class: "input",
        placeholder: "Search",
        type: "text",
        value: $searchInput
      },
      handler: {
        click: focusSearch,
        input: searchLogs.prepend(e => e.target.value)
      },
    })

    h('img', {
      visible: $searchInput.map(v => v.length > 0),
      attr: {
        class: "close-icon",
        src: "/content/assets/img/icons/close.svg"
      },
      handler: {click: clearSearch}
    })

  })
}

export { Search };