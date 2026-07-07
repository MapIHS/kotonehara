<script>
</script>

<svelte:head>
  <title>Kotonehara - Help</title>
</svelte:head>

<div class="page">
  <div class="page-header">
    <h1>Help</h1>
    <p>Panduan dan informasi tentang Kotonehara</p>
  </div>

  <div class="cards-grid">
    <div class="help-card">
      <div class="help-icon">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2L2 7l10 5 10-5-10-5z"/>
          <path d="M2 17l10 5 10-5"/>
          <path d="M2 12l10 5 10-5"/>
        </svg>
      </div>
      <h2>Tentang Kotonehara</h2>
      <p>Bot WhatsApp berbasis Go yang dibangun dengan <strong>whatsmeow</strong>. Menyediakan command untuk downloader, sticker, AI chat, dan lainnya.</p>
    </div>

    <div class="help-card">
      <div class="help-icon blue">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="4 17 10 11 4 5"/>
          <line x1="12" y1="19" x2="20" y2="19"/>
        </svg>
      </div>
      <h2>Commands</h2>
      <p>Gunakan prefix <strong>.</strong> (titik) untuk menjalankan command. Contoh: <code>.ping</code>, <code>.sticker</code>, <code>.ai</code></p>
    </div>

    <div class="help-card">
      <div class="help-icon green">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
          <polyline points="14 2 14 8 20 8"/>
          <line x1="16" y1="13" x2="8" y2="13"/>
          <line x1="16" y1="17" x2="8" y2="17"/>
          <polyline points="10 9 9 9 8 9"/>
        </svg>
      </div>
      <h2>Konfigurasi</h2>
      <p>Semua konfigurasi diatur lewat file <code>.env</code>. Lihat <code>.env.sample</code> untuk contoh variabel yang tersedia.</p>
    </div>

    <div class="help-card">
      <div class="help-icon orange">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
        </svg>
      </div>
      <h2>Health Check</h2>
      <p>Endpoint <code>/healthz</code> selalu publik dan bisa digunakan untuk monitoring. Dashboard dilindungi session cookie.</p>
    </div>
  </div>
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

  .cards-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 20px;
  }

  .help-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 28px;
    box-shadow: var(--shadow-sm);
    transition: all 0.2s ease;
  }

  .help-card:hover {
    box-shadow: var(--shadow-md);
    transform: translateY(-2px);
  }

  .help-icon {
    width: 48px;
    height: 48px;
    border-radius: 12px;
    background: var(--primary-light);
    color: var(--primary);
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 16px;
  }

  .help-icon.blue { background: var(--blue-light); color: var(--blue); }
  .help-icon.green { background: var(--green-light); color: var(--green); }
  .help-icon.orange { background: #fff3e0; color: #f57c00; }

  .help-card h2 {
    margin: 0 0 8px 0;
    font-size: 17px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .help-card p {
    margin: 0;
    font-size: 14px;
    color: var(--text-secondary);
    line-height: 1.6;
  }

  .help-card code {
    background: var(--bg-page);
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 13px;
    font-family: 'SF Mono', 'Fira Code', monospace;
    color: var(--primary);
  }

  @media (max-width: 768px) {
    .page {
      margin-left: 72px;
      padding: 24px 16px;
    }
    .cards-grid { grid-template-columns: 1fr; }
  }
</style>
