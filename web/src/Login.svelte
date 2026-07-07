<script>
  let { handleLogin = () => {} } = $props()

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let submitting = $state(false)

  async function onSubmit() {
    submitting = true
    error = ''
    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) {
        throw new Error(data.error || 'Login gagal.')
      }
      password = ''
      handleLogin?.()
    } catch (err) {
      error = err?.message ?? 'Login gagal.'
    } finally {
      submitting = false
    }
  }
</script>

<svelte:head>
  <title>Kotonehara - Login</title>
</svelte:head>

<main class="login-container">
  <div class="login-card">
    <div class="login-logo">
      <div class="logo-circle">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2L2 7l10 5 10-5-10-5z"/>
          <path d="M2 17l10 5 10-5"/>
          <path d="M2 12l10 5 10-5"/>
        </svg>
      </div>
    </div>
    <div class="login-header">
      <h1>Kotonehara</h1>
      <p>Bot WhatsApp Dashboard</p>
    </div>

    <form class="login-form" onsubmit={(e) => { e.preventDefault(); onSubmit(); }}>
      <div class="form-group">
        <label for="username">Username</label>
        <input
          id="username"
          type="text"
          bind:value={username}
          autocomplete="username"
          placeholder="Masukkan username"
          required
          disabled={submitting}
        />
      </div>

      <div class="form-group">
        <label for="password">Password</label>
        <input
          id="password"
          type="password"
          bind:value={password}
          autocomplete="current-password"
          placeholder="Masukkan password"
          required
          disabled={submitting}
        />
      </div>

      {#if error}
        <div class="error-message">
          <span class="icon">⚠️</span>
          <span>{error}</span>
        </div>
      {/if}

      <button
        type="submit"
        class="login-button"
        disabled={submitting || !username || !password}
      >
        {submitting ? 'Sedang masuk...' : 'Login'}
      </button>
    </form>

    <p class="footer-text">
      Dashboard dilindungi dengan session login. <br />
      Gunakan credential yang diset lewat .env.
    </p>
  </div>
</main>

<style>
  .login-container {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    padding: 16px;
    background: var(--bg-page);
  }

  .login-card {
    width: 100%;
    max-width: 400px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 40px;
    box-shadow: var(--shadow-md);
  }

  .login-logo {
    display: flex;
    justify-content: center;
    margin-bottom: 16px;
  }

  .logo-circle {
    width: 56px;
    height: 56px;
    background: var(--primary);
    border-radius: 16px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: white;
  }

  .login-header {
    text-align: center;
    margin-bottom: 32px;
  }

  .login-header h1 {
    margin: 0 0 8px 0;
    font-size: 26px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .login-header p {
    margin: 0;
    font-size: 14px;
    color: var(--text-secondary);
  }

  .login-form {
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .form-group label {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .form-group input {
    padding: 10px 14px;
    background: var(--bg-page);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-size: 14px;
    transition: all 0.2s ease;
  }

  .form-group input:focus {
    outline: none;
    border-color: var(--primary);
    box-shadow: 0 0 0 3px rgba(89, 50, 234, 0.1);
  }

  .form-group input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .form-group input::placeholder {
    color: var(--text-muted);
  }

  .error-message {
    display: flex;
    gap: 10px;
    padding: 12px;
    background: var(--red-light);
    border: 1px solid #f8c4c4;
    border-radius: var(--radius-sm);
    color: var(--red);
    font-size: 14px;
    align-items: center;
  }

  .error-message .icon {
    flex-shrink: 0;
  }

  .login-button {
    padding: 12px 16px;
    background: var(--primary);
    border: none;
    border-radius: var(--radius-sm);
    color: white;
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    margin-top: 4px;
  }

  .login-button:hover:not(:disabled) {
    background: #4a28d4;
    transform: translateY(-2px);
    box-shadow: 0 6px 16px rgba(89, 50, 234, 0.3);
  }

  .login-button:active:not(:disabled) {
    transform: translateY(0);
  }

  .login-button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .footer-text {
    text-align: center;
    font-size: 12px;
    color: var(--text-muted);
    margin: 24px 0 0 0;
    line-height: 1.5;
  }

  @media (max-width: 480px) {
    .login-card {
      padding: 32px 24px;
    }

    .login-header h1 {
      font-size: 22px;
    }
  }
</style>
