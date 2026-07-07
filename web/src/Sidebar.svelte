<script>
  let { activePage = 'dashboard', onNavigate = () => {}, onLogout = () => {} } = $props()
</script>

<aside class="sidebar">
  <div class="sidebar-logo">
    <div class="logo-icon">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 2L2 7l10 5 10-5-10-5z"/>
        <path d="M2 17l10 5 10-5"/>
        <path d="M2 12l10 5 10-5"/>
      </svg>
    </div>
    <div class="logo-text">
      <span class="logo-name">Kotonehara</span>
      <span class="logo-version">v1.0</span>
    </div>
  </div>

  <nav class="sidebar-nav">
    <button
      class="nav-item"
      class:active={activePage === 'dashboard'}
      onclick={() => onNavigate('dashboard')}
    >
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="3" y="3" width="7" height="7" rx="1"/>
        <rect x="14" y="3" width="7" height="7" rx="1"/>
        <rect x="3" y="14" width="7" height="7" rx="1"/>
        <rect x="14" y="14" width="7" height="7" rx="1"/>
      </svg>
      <span>Dashboard</span>
      <svg class="chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6"/>
      </svg>
    </button>

    <button
      class="nav-item"
      class:active={activePage === 'status'}
      onclick={() => onNavigate('status')}
    >
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M22 12h-4l-3 9L9 3l-3 9H2"/>
      </svg>
      <span>Status</span>
      <svg class="chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6"/>
      </svg>
    </button>

    <button
      class="nav-item"
      class:active={activePage === 'health'}
      onclick={() => onNavigate('health')}
    >
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
      </svg>
      <span>Health</span>
      <svg class="chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6"/>
      </svg>
    </button>

    <button
      class="nav-item"
      class:active={activePage === 'help'}
      onclick={() => onNavigate('help')}
    >
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="10"/>
        <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/>
        <line x1="12" y1="17" x2="12.01" y2="17"/>
      </svg>
      <span>Help</span>
      <svg class="chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="9 18 15 12 9 6"/>
      </svg>
    </button>

    <div class="nav-spacer"></div>

    <button class="nav-item nav-logout" onclick={onLogout}>
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
        <polyline points="16 17 21 12 16 7"/>
        <line x1="21" y1="12" x2="9" y2="12"/>
      </svg>
      <span>Logout</span>
    </button>
  </nav>
</aside>

<style>
  .sidebar {
    width: 250px;
    min-height: 100vh;
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    padding: 24px 16px;
    position: fixed;
    top: 0;
    left: 0;
    z-index: 10;
  }

  .sidebar-logo {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 12px;
    margin-bottom: 40px;
  }

  .logo-icon {
    width: 36px;
    height: 36px;
    background: var(--primary);
    border-radius: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
  }

  .logo-name {
    font-size: 18px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .logo-version {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-muted);
    margin-left: 4px;
  }

  .sidebar-nav {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex: 1;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    border: none;
    background: transparent;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-secondary);
    transition: all 0.2s ease;
    text-align: left;
    width: 100%;
  }

  .nav-item span {
    flex: 1;
  }

  .nav-item .chevron {
    opacity: 0.4;
    transition: opacity 0.2s ease;
  }

  .nav-item:hover {
    background: var(--primary-light);
    color: var(--primary);
  }

  .nav-item:hover .chevron {
    opacity: 0.8;
  }

  .nav-item.active {
    background: var(--primary);
    color: white;
  }

  .nav-item.active .chevron {
    opacity: 1;
  }

  .nav-spacer {
    flex: 1;
  }

  .nav-logout {
    color: var(--red);
  }

  .nav-logout:hover {
    background: var(--red-light);
    color: var(--red);
  }

  @media (max-width: 768px) {
    .sidebar {
      width: 72px;
      padding: 24px 8px;
    }

    .sidebar-logo .logo-text {
      display: none;
    }

    .nav-item span,
    .nav-item .chevron {
      display: none;
    }

    .nav-item {
      justify-content: center;
      padding: 12px;
    }
  }
</style>
