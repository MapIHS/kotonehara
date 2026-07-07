<script>
  import { onMount } from 'svelte'
  import Login from './Login.svelte'
  import Dashboard from './Dashboard.svelte'
  import Sidebar from './Sidebar.svelte'
  import StatusPage from './StatusPage.svelte'
  import HealthPage from './HealthPage.svelte'
  import HelpPage from './HelpPage.svelte'

  let session = $state({
    auth_enabled: false,
    authenticated: false,
    expires_at: '',
  })

  let status = $state(null)
  let loading = $state(true)
  let activePage = $state('dashboard')

  async function fetchSession() {
    try {
      const res = await fetch('/api/session', { credentials: 'include' })
      return await res.json()
    } catch (err) {
      console.error('fetchSession error:', err)
      return null
    }
  }

  async function fetchStatus() {
    try {
      const res = await fetch('/api/status', { credentials: 'include' })
      if (res.status === 401) {
        return null
      }
      if (!res.ok) {
        throw new Error('Gagal memuat status.')
      }
      return await res.json()
    } catch (err) {
      console.error('fetchStatus error:', err)
      return null
    }
  }

  async function boot() {
    loading = true
    try {
      const sess = await fetchSession()
      if (sess) {
        session = sess
        if (session.authenticated) {
          const st = await fetchStatus()
          if (st) {
            status = st
          }
        } else {
          status = null
        }
      }
    } catch (err) {
      console.error('Boot error:', err)
      status = null
    } finally {
      loading = false
    }
  }

  async function handleLogout() {
    await fetch('/api/logout', {
      method: 'POST',
      credentials: 'include',
    })
    await boot()
  }

  async function handleLogin() {
    await boot()
  }

  function handleNavigate(page) {
    activePage = page
  }

  onMount(() => {
    boot()
  })
</script>

{#if loading}
  <div class="loading-screen">
    <div class="loading-spinner"></div>
    <span>Memuat...</span>
  </div>
{:else if session.auth_enabled && !session.authenticated}
  <Login {handleLogin} />
{:else}
  <div class="app-layout">
    <Sidebar {activePage} onNavigate={handleNavigate} onLogout={handleLogout} />
    {#if activePage === 'dashboard'}
      <Dashboard bind:status {handleLogout} />
    {:else if activePage === 'status'}
      <StatusPage />
    {:else if activePage === 'health'}
      <HealthPage />
    {:else if activePage === 'help'}
      <HelpPage />
    {/if}
  </div>
{/if}

<style>
  .app-layout {
    display: flex;
    min-height: 100vh;
  }
</style>
