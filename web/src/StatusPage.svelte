<script>
  import { onMount } from 'svelte'

  let statusData = $state(null)
  let loading = $state(true)
  let error = $state('')
  let polling = null

  async function fetchStatus() {
    try {
      const res = await fetch('/api/status', { credentials: 'include' })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      statusData = await res.json()
      error = ''
    } catch (err) {
      error = err?.message ?? 'Gagal memuat status.'
    } finally {
      loading = false
    }
  }

  function uptime(startedAt) {
    if (!startedAt) return '-'
    const diff = Date.now() - new Date(startedAt).getTime()
    const seconds = Math.max(0, Math.floor(diff / 1000))
    const minutes = Math.floor(seconds / 60)
    const hours = Math.floor(minutes / 60)
    const days = Math.floor(hours / 24)
    if (days > 0) return `${days}d ${hours % 24}h ${minutes % 60}m`
    if (hours > 0) return `${hours}h ${minutes % 60}m ${seconds % 60}s`
    if (minutes > 0) return `${minutes}m ${seconds % 60}s`
    return `${seconds}s`
  }

  const dateFmt = new Intl.DateTimeFormat('id-ID', {
    dateStyle: 'full',
    timeStyle: 'medium',
  })

  onMount(() => {
    fetchStatus()
    polling = setInterval(fetchStatus, 5000)
    return () => { if (polling) clearInterval(polling) }
  })
</script>

<svelte:head>
  <title>Kotonehara - Status</title>
</svelte:head>

<div class="page">
  <div class="page-header">
    <h1>Status</h1>
    <p>Detail lengkap status bot WhatsApp</p>
  </div>

  {#if loading}
    <div class="loading-state">
      <div class="loading-spinner"></div>
      <span>Memuat status...</span>
    </div>
  {:else if error && !statusData}
    <div class="error-card">
      <span class="error-icon">⚠️</span>
      <span>{error}</span>
    </div>
  {:else if statusData}
    <div class="cards-grid">
      <div class="info-card">
        <div class="card-icon green">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
          </svg>
        </div>
        <div class="card-body">
          <span class="card-label">Stage</span>
          <span class="card-value">{statusData.stage}</span>
        </div>
        <span class="card-badge active">Active</span>
      </div>

      <div class="info-card">
        <div class="card-icon" class:green={statusData.connected} class:red={!statusData.connected}>
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M5 12.55a11 11 0 0 1 14.08 0"/>
            <path d="M1.42 9a16 16 0 0 1 21.16 0"/>
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
            <line x1="12" y1="20" x2="12.01" y2="20"/>
          </svg>
        </div>
        <div class="card-body">
          <span class="card-label">Connection</span>
          <span class="card-value">{statusData.connected ? 'Connected' : 'Disconnected'}</span>
        </div>
        {#if statusData.connected}
          <span class="card-badge active">Online</span>
        {:else}
          <span class="card-badge inactive">Offline</span>
        {/if}
      </div>

      <div class="info-card">
        <div class="card-icon" class:green={statusData.logged_in} class:red={!statusData.logged_in}>
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
            <circle cx="12" cy="7" r="4"/>
          </svg>
        </div>
        <div class="card-body">
          <span class="card-label">WhatsApp Login</span>
          <span class="card-value">{statusData.logged_in ? 'Logged In' : 'Not Logged In'}</span>
        </div>
        {#if statusData.logged_in}
          <span class="card-badge active">Active</span>
        {:else}
          <span class="card-badge inactive">Inactive</span>
        {/if}
      </div>

      <div class="info-card">
        <div class="card-icon blue">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
            <line x1="8" y1="21" x2="16" y2="21"/>
            <line x1="12" y1="17" x2="12" y2="21"/>
          </svg>
        </div>
        <div class="card-body">
          <span class="card-label">JID</span>
          <span class="card-value mono">{statusData.jid || '-'}</span>
        </div>
        <span class="card-badge neutral">Info</span>
      </div>

      <div class="info-card">
        <div class="card-icon green">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <polyline points="12 6 12 12 16 14"/>
          </svg>
        </div>
        <div class="card-body">
          <span class="card-label">Uptime</span>
          <span class="card-value">{uptime(statusData.started_at)}</span>
        </div>
        <span class="card-badge active">Running</span>
      </div>

      <div class="info-card">
        <div class="card-icon blue">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
            <line x1="16" y1="2" x2="16" y2="6"/>
            <line x1="8" y1="2" x2="8" y2="6"/>
            <line x1="3" y1="10" x2="21" y2="10"/>
          </svg>
        </div>
        <div class="card-body">
          <span class="card-label">Started At</span>
          <span class="card-value">{dateFmt.format(new Date(statusData.started_at))}</span>
        </div>
        <span class="card-badge neutral">Info</span>
      </div>

      {#if statusData.last_error}
        <div class="info-card card-error-highlight">
          <div class="card-icon red">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"/>
              <line x1="15" y1="9" x2="9" y2="15"/>
              <line x1="9" y1="9" x2="15" y2="15"/>
            </svg>
          </div>
          <div class="card-body">
            <span class="card-label">Last Error</span>
            <span class="card-value error-text">{statusData.last_error}</span>
          </div>
          <span class="card-badge inactive">Error</span>
        </div>
      {/if}
    </div>

    <div class="raw-section">
      <h2>Raw JSON</h2>
      <pre><code>{JSON.stringify(statusData, null, 2)}</code></pre>
    </div>
  {/if}
</div>

<style>
  .page {
    flex: 1;
    margin-left: 250px;
    padding: 32px 40px;
    min-height: 100vh;
  }

  .page-header {
    margin-bottom: 32px;
  }

  .page-header h1 {
    margin: 0;
    font-size: 26px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .page-header p {
    margin: 4px 0 0 0;
    font-size: 14px;
    color: var(--text-secondary);
  }

  .loading-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 300px;
    gap: 16px;
    color: var(--text-secondary);
  }

  .error-card {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 16px 20px;
    background: var(--red-light);
    border: 1px solid #f8c4c4;
    border-radius: var(--radius);
    color: var(--red);
    font-size: 14px;
  }

  .cards-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 16px;
    margin-bottom: 32px;
  }

  .info-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 20px;
    display: flex;
    align-items: center;
    gap: 16px;
    box-shadow: var(--shadow-sm);
    transition: all 0.2s ease;
  }

  .info-card:hover {
    box-shadow: var(--shadow-md);
    transform: translateY(-2px);
  }

  .card-error-highlight {
    border-color: #f8c4c4;
    background: #fffbfb;
  }

  .card-icon {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .card-icon.green { background: var(--green-light); color: var(--green); }
  .card-icon.red { background: var(--red-light); color: var(--red); }
  .card-icon.blue { background: var(--blue-light); color: var(--blue); }

  .card-body {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .card-label {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .card-value {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
    text-transform: capitalize;
    word-break: break-all;
  }

  .card-value.mono {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 13px;
    text-transform: none;
  }

  .error-text { color: var(--red); }

  .card-badge {
    display: inline-block;
    padding: 4px 12px;
    border-radius: 20px;
    font-size: 11px;
    font-weight: 600;
    flex-shrink: 0;
  }

  .card-badge.active { background: var(--green-light); color: var(--green); border: 1px solid #b2f0e0; }
  .card-badge.inactive { background: var(--red-light); color: var(--red); border: 1px solid #f8c4c4; }
  .card-badge.neutral { background: var(--blue-light); color: var(--blue); border: 1px solid #c4dafc; }

  .raw-section {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-sm);
    overflow: hidden;
  }

  .raw-section h2 {
    margin: 0;
    padding: 16px 20px;
    font-size: 16px;
    font-weight: 700;
    color: var(--text-primary);
    border-bottom: 1px solid var(--border);
  }

  .raw-section pre {
    margin: 0;
    padding: 20px;
    overflow-x: auto;
    font-size: 13px;
    color: var(--text-secondary);
    font-family: 'SF Mono', 'Fira Code', monospace;
    line-height: 1.6;
  }

  @media (max-width: 768px) {
    .page {
      margin-left: 72px;
      padding: 24px 16px;
    }
    .cards-grid { grid-template-columns: 1fr; }
  }
</style>
