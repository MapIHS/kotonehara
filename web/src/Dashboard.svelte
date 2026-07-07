<script>
  import { onMount } from 'svelte'
  import QRCode from 'qrcode'

  let { status = $bindable(null), handleLogout = () => {} } = $props()

  let polling = null
  let qrData = $state(null)
  let qrImageSrc = $state('')
  let qrPolling = null

  async function fetchStatus() {
    try {
      const res = await fetch('/api/status', { credentials: 'include' })
      if (res.status === 401) {
        handleLogout?.()
        return
      }
      if (!res.ok) return
      const data = await res.json()
      status = data
    } catch (err) {
      console.error('fetchStatus:', err)
    }
  }

  async function fetchQR() {
    try {
      const res = await fetch('/api/qr', { credentials: 'include' })
      if (!res.ok) return
      qrData = await res.json()
      if (qrData.qr_code) {
        qrImageSrc = await QRCode.toDataURL(qrData.qr_code, {
          width: 280,
          margin: 2,
          color: { dark: '#1a1a2e', light: '#ffffff' },
        })
      } else {
        qrImageSrc = ''
      }
    } catch (err) {
      console.error('fetchQR:', err)
    }
  }

  function uptime(startedAt) {
    if (!startedAt) return '-'
    const diff = Date.now() - new Date(startedAt).getTime()
    const seconds = Math.max(0, Math.floor(diff / 1000))
    const minutes = Math.floor(seconds / 60)
    const hours = Math.floor(minutes / 60)
    const days = Math.floor(hours / 24)
    if (days > 0) return `${days}d ${hours % 24}h`
    if (hours > 0) return `${hours}h ${minutes % 60}m`
    if (minutes > 0) return `${minutes}m ${seconds % 60}s`
    return `${seconds}s`
  }

  onMount(() => {
    polling = setInterval(fetchStatus, 5000)
    fetchQR()
    qrPolling = setInterval(fetchQR, 3000)
    return () => {
      if (polling) clearInterval(polling)
      if (qrPolling) clearInterval(qrPolling)
    }
  })
</script>

<svelte:head>
  <title>Kotonehara - Dashboard</title>
  <meta name="description" content="Dashboard Kotonehara untuk memantau status bot WhatsApp." />
</svelte:head>

<div class="dashboard-main">
  <header class="topbar">
    <div>
      <h1 class="greeting">Hello 👋</h1>
      <p class="greeting-sub">Selamat datang di Kotonehara Dashboard</p>
    </div>
  </header>

  {#if status}
    <!-- QR Code Card - shown when not logged in -->
    {#if !status.logged_in || (qrData && qrData.qr_code)}
      <div class="qr-section">
        <div class="qr-card">
          <div class="qr-left">
            <div class="qr-icon">
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="2" y="2" width="8" height="8" rx="1"/>
                <rect x="14" y="2" width="8" height="8" rx="1"/>
                <rect x="2" y="14" width="8" height="8" rx="1"/>
                <rect x="14" y="14" width="4" height="4" rx="0.5"/>
                <line x1="22" y1="14" x2="22" y2="22"/>
                <line x1="14" y1="22" x2="22" y2="22"/>
              </svg>
            </div>
            <h2>WhatsApp QR Login</h2>
            <p>Scan QR code ini dari WhatsApp untuk menghubungkan bot.</p>
            <div class="qr-status-info">
              <span class="qr-stage">
                <span class="dot" class:pulse={qrData?.qr_code}></span>
                {status.stage === 'waiting for qr login' ? 'Menunggu scan...' : status.stage}
              </span>
            </div>
          </div>
          <div class="qr-right">
            {#if qrImageSrc}
              <div class="qr-image-wrapper">
                <img src={qrImageSrc} alt="WhatsApp QR Code" class="qr-image" />
              </div>
              <span class="qr-hint">QR akan refresh otomatis</span>
            {:else if status.logged_in}
              <div class="qr-connected">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
                  <polyline points="22 4 12 14.01 9 11.01"/>
                </svg>
                <span>Sudah terhubung!</span>
              </div>
            {:else}
              <div class="qr-waiting">
                <div class="loading-spinner"></div>
                <span>Menunggu QR code...</span>
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Summary Cards -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon" class:green={status.connected} class:red={!status.connected}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M5 12.55a11 11 0 0 1 14.08 0"/>
            <path d="M1.42 9a16 16 0 0 1 21.16 0"/>
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0"/>
            <line x1="12" y1="20" x2="12.01" y2="20"/>
          </svg>
        </div>
        <div class="stat-info">
          <span class="stat-label">Connection</span>
          <span class="stat-value">{status.connected ? 'Connected' : 'Disconnected'}</span>
        </div>
        {#if status.connected}
          <span class="stat-badge active">Online</span>
        {:else}
          <span class="stat-badge inactive">Offline</span>
        {/if}
      </div>

      <div class="stat-card">
        <div class="stat-icon" class:green={status.logged_in} class:red={!status.logged_in}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
            <circle cx="12" cy="7" r="4"/>
          </svg>
        </div>
        <div class="stat-info">
          <span class="stat-label">WhatsApp</span>
          <span class="stat-value">{status.logged_in ? 'Logged In' : 'Not Logged In'}</span>
        </div>
        {#if status.logged_in}
          <span class="stat-badge active">Active</span>
        {:else}
          <span class="stat-badge inactive">Inactive</span>
        {/if}
      </div>

      <div class="stat-card">
        <div class="stat-icon green">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <polyline points="12 6 12 12 16 14"/>
          </svg>
        </div>
        <div class="stat-info">
          <span class="stat-label">Uptime</span>
          <span class="stat-value">{uptime(status.started_at)}</span>
        </div>
        <span class="stat-badge neutral">{status.stage}</span>
      </div>
    </div>
  {:else}
    <div class="loading-state">
      <div class="loading-spinner"></div>
      <span>Memuat dashboard...</span>
    </div>
  {/if}
</div>

<style>
  .dashboard-main {
    flex: 1;
    margin-left: 250px;
    padding: 32px 40px;
    min-height: 100vh;
  }

  .topbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 32px;
  }

  .greeting {
    margin: 0;
    font-size: 26px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .greeting-sub {
    margin: 4px 0 0 0;
    font-size: 14px;
    color: var(--text-secondary);
  }

  /* QR Section */
  .qr-section {
    margin-bottom: 28px;
  }

  .qr-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 32px;
    box-shadow: var(--shadow-sm);
    display: flex;
    gap: 40px;
    align-items: center;
  }

  .qr-left {
    flex: 1;
  }

  .qr-icon {
    width: 52px;
    height: 52px;
    border-radius: 14px;
    background: var(--primary-light);
    color: var(--primary);
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 16px;
  }

  .qr-left h2 {
    margin: 0 0 8px 0;
    font-size: 20px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .qr-left p {
    margin: 0 0 16px 0;
    font-size: 14px;
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .qr-status-info {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .qr-stage {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-secondary);
    text-transform: capitalize;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-muted);
  }

  .dot.pulse {
    background: var(--green);
    animation: pulse-glow 1.5s ease-in-out infinite;
  }

  @keyframes pulse-glow {
    0%, 100% { box-shadow: 0 0 0 0 rgba(0, 201, 167, 0.4); }
    50% { box-shadow: 0 0 0 6px rgba(0, 201, 167, 0); }
  }

  .qr-right {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }

  .qr-image-wrapper {
    background: white;
    border-radius: var(--radius);
    padding: 8px;
    border: 2px solid var(--border);
    box-shadow: var(--shadow-md);
  }

  .qr-image {
    display: block;
    width: 220px;
    height: 220px;
    border-radius: 8px;
  }

  .qr-hint {
    font-size: 11px;
    color: var(--text-muted);
  }

  .qr-connected {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    color: var(--green);
    padding: 40px;
  }

  .qr-connected span {
    font-size: 14px;
    font-weight: 600;
  }

  .qr-waiting {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 40px;
    color: var(--text-muted);
  }

  .qr-waiting span {
    font-size: 13px;
  }

  /* Stats Row */
  .stats-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 16px;
  }

  .stat-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 20px;
    display: flex;
    align-items: center;
    gap: 14px;
    box-shadow: var(--shadow-sm);
    transition: all 0.2s ease;
  }

  .stat-card:hover {
    box-shadow: var(--shadow-md);
    transform: translateY(-1px);
  }

  .stat-icon {
    width: 42px;
    height: 42px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .stat-icon.green { background: var(--green-light); color: var(--green); }
  .stat-icon.red { background: var(--red-light); color: var(--red); }

  .stat-info {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .stat-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-muted);
  }

  .stat-value {
    font-size: 16px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .stat-badge {
    padding: 4px 10px;
    border-radius: 20px;
    font-size: 11px;
    font-weight: 600;
    flex-shrink: 0;
    text-transform: capitalize;
  }

  .stat-badge.active { background: var(--green-light); color: var(--green); border: 1px solid #b2f0e0; }
  .stat-badge.inactive { background: var(--red-light); color: var(--red); border: 1px solid #f8c4c4; }
  .stat-badge.neutral { background: var(--blue-light); color: var(--blue); border: 1px solid #c4dafc; }

  /* Loading */
  .loading-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 400px;
    gap: 16px;
    color: var(--text-secondary);
  }

  @media (max-width: 768px) {
    .dashboard-main {
      margin-left: 72px;
      padding: 24px 16px;
    }

    .greeting {
      font-size: 22px;
    }

    .qr-card {
      flex-direction: column;
      padding: 24px;
      gap: 24px;
    }

    .stats-row {
      grid-template-columns: 1fr;
    }
  }
</style>
