<script>
  import { onMount } from 'svelte'

  let healthResult = $state(null)
  let statusResult = $state(null)
  let loading = $state(true)
  let polling = null

  async function fetchAll() {
    const results = await Promise.allSettled([
      fetch('/healthz').then(async (r) => ({ ok: r.ok, status: r.status, body: await r.text(), url: '/healthz' })),
      fetch('/api/status', { credentials: 'include' }).then(async (r) => ({ ok: r.ok, status: r.status, body: await r.json().catch(() => null), url: '/api/status' })),
    ])

    healthResult = results[0].status === 'fulfilled' ? results[0].value : { ok: false, status: 0, body: results[0].reason?.message ?? 'Error', url: '/healthz' }
    statusResult = results[1].status === 'fulfilled' ? results[1].value : { ok: false, status: 0, body: null, url: '/api/status' }
    loading = false
  }

  onMount(() => {
    fetchAll()
    polling = setInterval(fetchAll, 10000)
    return () => { if (polling) clearInterval(polling) }
  })
</script>

<svelte:head>
  <title>Kotonehara - Health</title>
</svelte:head>

<div class="page">
  <div class="page-header">
    <h1>Health</h1>
    <p>Health check endpoint dan monitoring bot</p>
  </div>

  {#if loading}
    <div class="loading-state">
      <div class="loading-spinner"></div>
      <span>Checking health...</span>
    </div>
  {:else}
    <div class="cards-grid">
      <!-- Health Check Card -->
      <div class="health-card" class:healthy={healthResult?.ok} class:unhealthy={!healthResult?.ok}>
        <div class="hc-header">
          <div class="hc-icon" class:green={healthResult?.ok} class:red={!healthResult?.ok}>
            {#if healthResult?.ok}
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                <polyline points="22 4 12 14.01 9 11.01"/>
              </svg>
            {:else}
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
              </svg>
            {/if}
          </div>
          <span class="hc-badge" class:active={healthResult?.ok} class:inactive={!healthResult?.ok}>
            {healthResult?.ok ? 'Healthy' : 'Unhealthy'}
          </span>
        </div>
        <h2 class="hc-title">Health Check</h2>
        <p class="hc-endpoint">
          <span class="method">GET</span>
          <code>/healthz</code>
        </p>
        <div class="hc-details">
          <div class="hc-detail">
            <span class="detail-label">Status Code</span>
            <span class="detail-value">{healthResult?.status ?? '-'}</span>
          </div>
          <div class="hc-detail">
            <span class="detail-label">Response</span>
            <span class="detail-value mono">{typeof healthResult?.body === 'string' ? healthResult.body.trim() : '-'}</span>
          </div>
        </div>
        <a href="/healthz" target="_blank" rel="noreferrer" class="hc-link">
          Buka endpoint →
        </a>
      </div>

      <!-- API Status Card -->
      <div class="health-card" class:healthy={statusResult?.ok} class:unhealthy={!statusResult?.ok}>
        <div class="hc-header">
          <div class="hc-icon" class:green={statusResult?.ok} class:red={!statusResult?.ok}>
            {#if statusResult?.ok}
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                <polyline points="22 4 12 14.01 9 11.01"/>
              </svg>
            {:else}
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
              </svg>
            {/if}
          </div>
          <span class="hc-badge" class:active={statusResult?.ok} class:inactive={!statusResult?.ok}>
            {statusResult?.ok ? 'OK' : 'Error'}
          </span>
        </div>
        <h2 class="hc-title">API Status</h2>
        <p class="hc-endpoint">
          <span class="method">GET</span>
          <code>/api/status</code>
        </p>
        <div class="hc-details">
          <div class="hc-detail">
            <span class="detail-label">Status Code</span>
            <span class="detail-value">{statusResult?.status ?? '-'}</span>
          </div>
          {#if statusResult?.body}
            <div class="hc-detail">
              <span class="detail-label">Stage</span>
              <span class="detail-value">{statusResult.body.stage ?? '-'}</span>
            </div>
            <div class="hc-detail">
              <span class="detail-label">Connected</span>
              <span class="detail-value">{statusResult.body.connected ? 'Yes' : 'No'}</span>
            </div>
            <div class="hc-detail">
              <span class="detail-label">Logged In</span>
              <span class="detail-value">{statusResult.body.logged_in ? 'Yes' : 'No'}</span>
            </div>
          {/if}
        </div>
        <a href="/status" target="_blank" rel="noreferrer" class="hc-link">
          Buka endpoint →
        </a>
      </div>

      <!-- Session Check Card -->
      <div class="health-card healthy">
        <div class="hc-header">
          <div class="hc-icon green">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
            </svg>
          </div>
          <span class="hc-badge active">Active</span>
        </div>
        <h2 class="hc-title">Session</h2>
        <p class="hc-endpoint">
          <span class="method">GET</span>
          <code>/api/session</code>
        </p>
        <div class="hc-details">
          <div class="hc-detail">
            <span class="detail-label">Status</span>
            <span class="detail-value">Authenticated</span>
          </div>
          <div class="hc-detail">
            <span class="detail-label">Type</span>
            <span class="detail-value">Cookie Session</span>
          </div>
        </div>
      </div>
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

  .cards-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 20px;
  }

  .health-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 24px;
    box-shadow: var(--shadow-sm);
    transition: all 0.2s ease;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .health-card:hover {
    box-shadow: var(--shadow-md);
    transform: translateY(-2px);
  }

  .health-card.healthy {
    border-left: 4px solid var(--green);
  }

  .health-card.unhealthy {
    border-left: 4px solid var(--red);
  }

  .hc-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .hc-icon {
    width: 52px;
    height: 52px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .hc-icon.green { background: var(--green-light); color: var(--green); }
  .hc-icon.red { background: var(--red-light); color: var(--red); }

  .hc-badge {
    padding: 4px 14px;
    border-radius: 20px;
    font-size: 12px;
    font-weight: 600;
  }

  .hc-badge.active { background: var(--green-light); color: var(--green); border: 1px solid #b2f0e0; }
  .hc-badge.inactive { background: var(--red-light); color: var(--red); border: 1px solid #f8c4c4; }

  .hc-title {
    margin: 0;
    font-size: 20px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .hc-endpoint {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
    font-size: 13px;
    color: var(--text-secondary);
  }

  .method {
    background: var(--blue-light);
    color: var(--blue);
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.5px;
  }

  .hc-endpoint code {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 13px;
  }

  .hc-details {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 0;
    border-top: 1px solid var(--border);
  }

  .hc-detail {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .detail-label {
    font-size: 13px;
    color: var(--text-muted);
  }

  .detail-value {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .detail-value.mono {
    font-family: 'SF Mono', 'Fira Code', monospace;
  }

  .hc-link {
    display: inline-block;
    font-size: 13px;
    font-weight: 600;
    color: var(--primary);
    text-decoration: none;
    transition: color 0.2s ease;
  }

  .hc-link:hover {
    color: #4a28d4;
  }

  @media (max-width: 768px) {
    .page {
      margin-left: 72px;
      padding: 24px 16px;
    }
    .cards-grid { grid-template-columns: 1fr; }
  }
</style>
