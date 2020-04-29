import {selectDate, updateDate, switchTimeRange} from './stores'

const calendar = document.getElementById("calendar-body")

const monthNames = ["January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December"
]
const monthAbbreviations = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]
let filledMonths = []
let currentMonthPosition

const showNextMonth = () => {
  currentMonthPosition += 100
  calendar.querySelectorAll('.month').forEach(e => e.style.transform = `translateX(-${currentMonthPosition}%)`)
}

const showPreviousMonth = () => {
  currentMonthPosition -= 100
  calendar.querySelectorAll('.month').forEach(e => e.style.transform = `translateX(-${currentMonthPosition}%)`)
}

const formatDate = (date) => {
  return `${(date.getDate()+1<10?'0':'')+date.getDate()}.${(date.getMonth()+1<10?'0':'')+parseInt(date.getMonth()+1)}.${String(date.getYear()+1900).substring(2)} ${(date.getHours()<10?'0':'')+date.getHours()}:${(date.getMinutes()<10?'0':'')+date.getMinutes()}`
}

export const updateCalendar = () => {
  let uniqueMonthYear = window.response_metrics
    .filter((v,i,a) => a
      .findIndex(t => (t.stamp.getMonth() === v.stamp.getMonth() && t.stamp.getYear() === v.stamp.getYear())) === i)
    .sort((i, j) => (i.stamp < j.stamp ? -1 : 1))

  let uniqueMonthDay = window.response_metrics
    .filter((v,i,a) => a
      .findIndex(t => (t.stamp.getMonth() === v.stamp.getMonth() && t.stamp.getDay() === v.stamp.getDay())) === i)
  let uniqueMonthDayMap = uniqueMonthDay.map(v => ({day: v.stamp.getDate(), month: v.stamp.getMonth()}))

  for (let i=0; i<uniqueMonthYear.length; i++) {
    let v = uniqueMonthYear[i]
    let thisMonth = v.stamp.getMonth()
    let thisYear = v.stamp.getYear()
    let monthName = monthNames[thisMonth]

    let daysInMonth = new Date(thisYear, thisMonth+1, 0).getDate()
    let startingPoint = new Date(thisYear, thisMonth, 1).getDay()+1

    let _month = document.createElement('div')
    let monthExist = filledMonths.filter(m => JSON.stringify(m) === JSON.stringify({month: v.stamp.getMonth(), year: v.stamp.getYear()+1900}))

    if (monthExist.length == 0) {
      _month.classList.add('month')
      _month.dataset.month = thisMonth
      _month.dataset.year = thisYear+1900

      let _monthLabel = document.createElement('div')
      _monthLabel.classList.add('month-label-wrapper')

      let _monthName = document.createElement('div')
      _monthName.classList.add('month-name')
      _monthName.innerText = `${monthName} ${thisYear+1900}`

      let _monthBack = document.createElement('div')
      _monthBack.classList.add("month-arrow")
      _monthBack.classList.add("month-back")
      _monthBack.innerText = '<'

      let _monthForward = document.createElement('div')
      _monthForward.classList.add("month-arrow")
      _monthForward.classList.add("month-forward")
      _monthForward.innerText = '>'

      _monthLabel.appendChild(_monthBack)
      _monthLabel.appendChild(_monthName)
      _monthLabel.appendChild(_monthForward)

      _month.appendChild(_monthLabel)
  
      let _daysHeader = document.createElement('div')
      _daysHeader.classList.add('daysheader')
      _month.appendChild(_daysHeader)
      
      monthAbbreviations.forEach(v => {
        let _day = document.createElement('div')
        _day.classList.add('day')
        _day.innerText = v
        _daysHeader.appendChild(_day)
      })
  
      for (let i=0; i<=Math.round(daysInMonth/7)+1; i++) {
        let _week = document.createElement('div')
        _week.classList.add('week')
        for (let j=0; j<7; j++) {
          let _cell = document.createElement('div')
          _cell.classList.add('cell')
          _cell.addEventListener('click', function(e) {
            if (!e.target.classList.contains('active')) return
            let root = document.querySelector('.calendar')
            if (_cell.classList.contains('selected')) {
              _cell.classList.remove('selected')
              selectDate({range: root.getAttribute('data-range')})
              let filter = document.querySelector('.date-picker .filter')
              filter.querySelector(`.filter-date.${root.getAttribute('data-range')}`).innerText = ''
              if (!window.date_ranges.from && !window.date_ranges.to) {
                document.querySelectorAll('.segmented-control label').forEach(v => v.classList.add('available'))
                window.custom_date_ranges = false
                filter.querySelector('.filter-label').classList.remove('altered')
                filter.querySelector('.filter-label').innerText = 'Filter date'
              }
              return
            }
            let hour = root.querySelector('.hour').value
            let minute = root.querySelector('.minute').value
            selectDate({range: root.getAttribute("data-range"), year: thisYear+1900, month: _cell.getAttribute("data-month"), day: _cell.innerText, hour: hour, minute: minute})
            calendar.querySelectorAll('.cell.selected').forEach(e => e.classList.remove('selected'))
            document.querySelectorAll('.segmented-control label.available').forEach(v => v.classList.remove('available'))
            _cell.classList.add('selected')
            let filter = document.querySelector('.date-picker .filter')
            filter.querySelector('.filter-label').classList.add('altered')
            filter.querySelector('.filter-label').innerText = '—'
            switch (root.getAttribute('data-range')) {
              case 'from':
                filter.querySelector('.filter-date.from').innerText = formatDate(window.date_ranges.from)
                break
              case 'to':
                filter.querySelector('.filter-date.to').innerText = formatDate(window.date_ranges.to)
                break
            }
          })
          _week.appendChild(_cell)
        }
        _month.appendChild(_week)
      }
  
      for (let i=1; i<=daysInMonth; i++) {
        if (startingPoint > 6) {startingPoint = 0}
        _month.dataset.startingPoint = startingPoint
        let cell = _month.getElementsByClassName('cell')[i+startingPoint-1]
        cell.dataset.month = thisMonth
        cell.innerText = i
      }
    
      let fragment = document.createDocumentFragment()
      fragment.appendChild(_month)
      calendar.querySelector(".months").appendChild(fragment)

      if (i+1 == uniqueMonthYear.length) {
        currentMonthPosition = i*100
        calendar.querySelectorAll('.month').forEach(e => e.style.transform = `translateX(-${currentMonthPosition}%)`)
      }

      filledMonths.push({month: thisMonth, year: thisYear+1900})
    }
    
    _month = calendar.querySelector(`.month[data-month="${thisMonth}"][data-year="${thisYear+1900}"]`)
    for (let i=1; i<=daysInMonth; i++) {
      let iDate = {day: i, month: thisMonth}
      if (startingPoint > 6) {startingPoint = 0}
      uniqueMonthDayMap.forEach(v => {
        if (JSON.stringify(v) === JSON.stringify(iDate)) {
          _month.getElementsByClassName('cell')[i+startingPoint-1].classList.add('active') 
        }
      })
    }

    if (i!=uniqueMonthYear.length-1 && uniqueMonthYear.length>1) {
      let _monthForward = _month.querySelector('.month-forward')
      _monthForward.classList.add('active')
      _monthForward.addEventListener('click', showNextMonth)
    }
    if (i!=0 && i<uniqueMonthYear.length) {
      let _monthBack = _month.querySelector('.month-back')
      _monthBack.classList.add('active')
      _monthBack.addEventListener('click', showPreviousMonth)
    }
  }

  const activeCells = calendar.querySelectorAll('.cell.active')
  if (activeCells.length > 0) {
    for (let j=0; j<activeCells.length; j++) {
      activeCells[j].classList.remove('current')
    }
    activeCells[activeCells.length-1].classList.add('current')
  }
}

export const updateChartRange = (range) => {
  let startStamp
  let stopStamp
  switch (range.range) {
    case "from":
      startStamp = null
      if (Object.keys(range).length > 1) {
        startStamp = new Date(range.year, range.month, range.day, range.hour, range.minute)
      }
      break
    case "to":
      stopStamp = null
      if (Object.keys(range).length > 1) {
        stopStamp = new Date(range.year, range.month, range.day, range.hour, range.minute)
      }
      break
  }

  window.custom_date_ranges = true
  switchTimeRange({
    from: startStamp, 
    to: stopStamp
  })
}

export const updateChartRangeHourMinute = (range, hour, minute) => {
  let filter = document.querySelector('.date-picker .filter')
  let startStamp
  let stopStamp
  let tempStamp
  if (hour) {
    tempStamp = window.date_ranges[range]
    if (tempStamp) tempStamp.setHours(hour)
  }
  if (minute) {
    tempStamp = window.date_ranges[range]
    if (tempStamp) tempStamp.setMinutes(minute)
  }
  switch (range) {
    case "from":
      startStamp = tempStamp
      break
    case "to":
      stopStamp = tempStamp
      break
  }
  
  window.custom_date_ranges = true
  switchTimeRange({
    from: startStamp, 
    to: stopStamp
  })

  filter.querySelector(`.filter-date.${range}`).innerText = formatDate(window.date_ranges[range])
}

export const closeCalendar = (t) => {
  t.classList.toggle("visible")
  document.getElementsByClassName('calendar')[0].classList.toggle("visible")
}

export const openCalendar = () => {
  document.getElementsByClassName('calendar')[0].classList.toggle("visible")
  document.getElementsByClassName('calendar-close')[0].classList.toggle("visible")
}

export const updateRange = (e) => {
  let root = document.querySelector('.calendar')
  switch (e.classList[0]) {
    case "hour":
      updateDate({range: root.getAttribute("data-range"), hour: e.value})
      break
    case "minute":
      updateDate({range: root.getAttribute("data-range"), minute: e.value})
      break
  }
}


export const selectRange = (target) => {
  document.querySelectorAll('.date-box').forEach(e => {
    e.classList.remove('active')
  })
  target.classList.add('active')
  let range = target.getAttribute("data-range")
  document.querySelector('.calendar').dataset.range = range

  let date = window.calendar_ranges[range]
  let _selectedCell = document.querySelector('.cell.selected')
  if (_selectedCell) {
    _selectedCell.classList.remove('selected')
  }
  let _times = document.querySelector('.calendar .time')
  _times.querySelector('.hour').value = ""
  _times.querySelector('.minute').value = ""
  if (Object.keys(date).length > 1) {
    let _month = document.querySelector(`.month[data-month="${date.month}"][data-year="${date.year}"]`)
    let startingPoint = _month.getAttribute("data-starting-point")
    let _cell = _month.querySelectorAll('.cell')[parseInt(date.day)+parseInt(startingPoint)-1]
    _cell.classList.add("selected")
    _times.querySelector('.hour').value = date.hour
    _times.querySelector('.minute').value = date.minute
  }
}