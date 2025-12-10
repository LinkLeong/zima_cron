const API_BASE = (window.API_BASE || '').replace(/\/$/, '')
let tasks = []

function simulate(message, failRate) {
  return new Promise((resolve) => {
    const ms = 600 + Math.floor(Math.random() * 1200)
    setTimeout(() => {
      const ok = Math.random() > failRate
      resolve({ success: ok, output: ok ? message : '执行失败' })
    }, ms)
  })
}

function fmtTime(ts) {
  if (!ts) return '-'
  const d = new Date(ts)
  const y = d.getFullYear()
  const m = `${d.getMonth() + 1}`.padStart(2, '0')
  const dd = `${d.getDate()}`.padStart(2, '0')
  const hh = `${d.getHours()}`.padStart(2, '0')
  const mm = `${d.getMinutes()}`.padStart(2, '0')
  const ss = `${d.getSeconds()}`.padStart(2, '0')
  return `${y}-${m}-${dd} ${hh}:${mm}:${ss}`
}

function byId(id) { return document.getElementById(id) }

function init() {
  const modal = byId('createTaskModal')
  const openBtn = byId('openCreateModalBtn')
  const cancelBtn = byId('cancelCreateBtn')
  const createBtn = byId('createTaskBtn')
  const scheduleTypeSelect = byId('scheduleTypeSelect')
  const rowInterval = byId('rowInterval')
  const rowCron = byId('rowCron')

  function openCreate() { modal.classList.add('show') }
  function closeCreate() { modal.classList.remove('show') }

  function updateScheduleInputVisibility() {
    const t = scheduleTypeSelect.value
    if (t === 'cron') {
      rowInterval.classList.add('hidden')
      rowCron.classList.remove('hidden')
    } else {
      rowInterval.classList.remove('hidden')
      rowCron.classList.add('hidden')
    }
  }

  openBtn.addEventListener('click', openCreate)
  cancelBtn.addEventListener('click', closeCreate)
  createBtn.addEventListener('click', () => { createTask(); closeCreate() })

  byId('runAllBtn').addEventListener('click', runAllOnce)
  scheduleTypeSelect.addEventListener('change', updateScheduleInputVisibility)
  updateScheduleInputVisibility()

  fetchTasks()
}

// removed script selection

async function createTask() {
  const name = byId('taskNameInput').value.trim()
  const command = byId('commandInput').value.trim()
  const scheduleType = byId('scheduleTypeSelect').value
  const intervalMin = parseInt(byId('intervalInput').value, 10)
  const cronExpr = byId('cronInput').value.trim()
  if (!name || !command) return
  if (scheduleType === 'interval') {
    if (!intervalMin || intervalMin < 1) return
  } else {
    if (!isValidCron(cronExpr)) return
  }
  const payload = { name, command, type: scheduleType, interval_min: intervalMin, cron_expr: cronExpr }
  await fetch(`${API_BASE}/zima_cron/tasks`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
  await fetchTasks()
  byId('taskNameInput').value = ''
  byId('commandInput').value = ''
  byId('intervalInput').value = ''
  byId('cronInput').value = ''
}

function startSchedule(task) {
  clearSchedule(task)
  if (task.type === 'interval') {
    task.timerType = 'interval'
    task.timer = setInterval(() => scheduleRun(task.id), task.intervalMs)
    task.nextRunAt = Date.now() + task.intervalMs
  } else {
    scheduleCronNext(task)
  }
}

function clearSchedule(task) {
  if (task.timer) {
    if (task.timerType === 'interval') clearInterval(task.timer)
    else clearTimeout(task.timer)
    task.timer = null
    task.timerType = null
  }
}

// scheduling handled by backend

// execution handled by backend

function renderTasks() {
  const tbody = byId('taskTableBody')
  tbody.innerHTML = ''
  tasks.forEach(t => {
    const tr = document.createElement('tr')
    const statusDot = `<span class="dot ${t.status}"></span>`
    const statusText = t.status === 'running' ? 'Running' : 'Paused'
    const lastBadge = t.lastResult ? `<span class="result-badge"><span class="dot ${t.lastResult.success ? 'success' : 'fail'}"></span>${t.lastResult.success ? 'Success' : 'Fail'}</span>` : '<span class="muted">-</span>'

    tr.innerHTML = `
      <td>${t.name}</td>
      <td><code>${escapeHtml(t.command)}</code></td>
      <td><span class="status">${statusDot}${statusText}</span></td>
      <td>${fmtTime(t.nextRunAt)}</td>
      <td>${lastBadge}</td>
      <td class="actions">
        <button data-action="run" data-id="${t.id}">Run Once</button>
        <button data-action="toggle" data-id="${t.id}">${t.status === 'running' ? 'Pause' : 'Resume'}</button>
        <button data-action="logs" data-id="${t.id}">${t.showLogs ? 'Hide Logs' : 'Show Logs'}</button>
      </td>
    `
    tbody.appendChild(tr)

    const logsRow = document.createElement('tr')
    const logsTd = document.createElement('td')
    logsTd.colSpan = 6
    if (t.showLogs) {
      const header = document.createElement('div')
      header.className = 'logs-header'
      header.innerHTML = `
        <div class="muted">Logs · ${t.name}</div>
        <div class="list-actions"><button data-action="clear-logs" data-id="${t.id}">Clear Logs</button></div>
      `
      const list = document.createElement('div')
      list.className = 'logs-list'
      (t.logs || []).slice(0, 100).forEach(l => {
        const item = document.createElement('div')
        item.className = 'log-item'
        const statusClass = l.success ? 'success' : 'fail'
        item.innerHTML = `
          <div class="log-time">${fmtTime(l.time)}</div>
          <div>${escapeHtml(l.message)}</div>
          <div class="log-status ${statusClass}">${l.success ? 'Success' : 'Fail'}</div>
        `
        list.appendChild(item)
      })
      const container = document.createElement('div')
      container.className = 'row-logs'
      container.appendChild(header)
      container.appendChild(list)
      logsTd.appendChild(container)
    }
    logsRow.appendChild(logsTd)
    tbody.appendChild(logsRow)
  })

  tbody.querySelectorAll('button').forEach(btn => btn.addEventListener('click', onRowAction))
}

async function onRowAction(e) {
  const action = e.currentTarget.getAttribute('data-action')
  const id = e.currentTarget.getAttribute('data-id')
  const task = tasks.find(t => t.id === id)
  if (!task) return
  if (action === 'run') fetch(`${API_BASE}/zima_cron/tasks/${id}/run`, { method: 'POST' }).then(fetchTasks)
  if (action === 'toggle') fetch(`${API_BASE}/zima_cron/tasks/${id}/toggle`, { method: 'POST' }).then(fetchTasks)
  if (action === 'logs') {
    task.showLogs = !task.showLogs
    if (task.showLogs && !task.logs) {
      try {
        const lr = await fetch(`${API_BASE}/zima_cron/tasks/${id}/logs`)
        task.logs = lr.ok ? await lr.json() : []
      } catch (_) {
        task.logs = []
      }
    }
    renderTasks()
  }
  if (action === 'clear-logs') { fetch(`${API_BASE}/zima_cron/tasks/${id}/logs/clear`, { method: 'POST' }).then(() => { task.logs = []; renderTasks() }) }
}

// backend handles toggle

function runAllOnce() {
  const running = tasks.filter(t => t.status === 'running')
  Promise.all(running.map(t => fetch(`${API_BASE}/zima_cron/tasks/${t.id}/run`, { method: 'POST' }))).then(fetchTasks)
}

function scheduleCronNext(task) {
  const now = new Date()
  const next = cronNext(task.cronExpr, now)
  if (!next) return
  const delay = next.getTime() - Date.now()
  task.timerType = 'timeout'
  task.timer = setTimeout(() => scheduleRun(task.id), delay)
  task.nextRunAt = next.getTime()
}

function isValidCron(expr) { const parts = expr.trim().split(/\s+/); return parts.length === 5 }

function cronNext(expr, fromDate) {
  const parts = expr.trim().split(/\s+/)
  if (parts.length !== 5) return null
  const [minExpr, hourExpr, domExpr, monExpr, dowExpr] = parts
  const minSet = parseCronField(minExpr, 0, 59)
  const hourSet = parseCronField(hourExpr, 0, 23)
  const monSet = parseCronField(monExpr, 1, 12)
  const domSet = parseCronField(domExpr, 1, 31)
  const dowSet = parseCronField(dowExpr, 0, 6, true)
  let d = new Date(fromDate.getTime())
  d.setSeconds(0); d.setMilliseconds(0)
  d = new Date(d.getTime() + 60000)
  for (let i = 0; i < 100000; i++) {
    const m = d.getMinutes()
    const h = d.getHours()
    const month = d.getMonth() + 1
    const day = d.getDate()
    let dow = d.getDay()
    if (dow === 0 && dowSet.has7) dow = 7
    const minuteOk = minSet.set.has(m)
    const hourOk = hourSet.set.has(h)
    const monthOk = monSet.set.has(month)
    const domOk = domSet.set.has(day)
    const dowOk = dowSet.set.has(dow)
    const dayOk = (domSet.isAll && dowSet.isAll) ? true
      : (domSet.isAll ? dowOk : (dowSet.isAll ? domOk : (domOk || dowOk)))
    if (minuteOk && hourOk && monthOk && dayOk) return d
    d = new Date(d.getTime() + 60000)
  }
  return null
}

function parseCronField(expr, min, max, isDow = false) {
  const set = new Set()
  const lower = expr.toLowerCase()
  const alias = { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 }
  let isAll = false
  let has7 = false
  const tokens = lower.split(',')
  const addRange = (start, end, step = 1) => {
    for (let v = start; v <= end; v += step) set.add(v)
  }
  tokens.forEach(tok => {
    tok = tok.trim()
    if (tok === '*') { isAll = true; addRange(min, max); return }
    const m = tok.match(/^\*(\/(\d+))?$/)
    if (m) { const step = m[2] ? parseInt(m[2], 10) : 1; isAll = true; addRange(min, max, step); return }
    const aliasVal = isDow ? alias[tok] : undefined
    if (aliasVal !== undefined) { set.add(aliasVal); return }
    if (tok.includes('-')) {
      const [a, bPart] = tok.split('-')
      let b = bPart
      let step = 1
      if (b.includes('/')) { const [bb, ss] = b.split('/'); b = bb; step = parseInt(ss, 10) }
      const start = parseInt(a, 10)
      let end = parseInt(b, 10)
      if (isDow && end === 7) { has7 = true }
      if (isNaN(start) || isNaN(end)) return
      addRange(Math.max(min, start), Math.min(max, end), step)
      return
    }
    let stepMatch = tok.match(/^(\d+)-(\d+)\/(\d+)$/)
    if (stepMatch) {
      const start = parseInt(stepMatch[1], 10)
      const end = parseInt(stepMatch[2], 10)
      const step = parseInt(stepMatch[3], 10)
      addRange(Math.max(min, start), Math.min(max, end), step)
      return
    }
    if (tok.includes('/')) {
      const [base, stepStr] = tok.split('/')
      if (base === '*') { const step = parseInt(stepStr, 10); addRange(min, max, step); return }
    }
    let v = parseInt(tok, 10)
    if (isDow && v === 7) { has7 = true }
    if (!isNaN(v)) {
      if (v >= min && v <= max) set.add(v)
    }
  })
  return { set, isAll, has7 }
}

// removed global logs panel

// logs clear handled per-row

function escapeHtml(str) { return str.replace(/[&<>"]/g, s => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[s])) }

async function fetchTasks() {
  const res = await fetch(`${API_BASE}/zima_cron/tasks`)
  const list = await res.json()
  tasks = list.map(t => ({ ...t, showLogs: false, logs: undefined }))
  renderTasks()
}

document.addEventListener('DOMContentLoaded', init)
