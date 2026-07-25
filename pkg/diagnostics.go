package pkg

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/MapIHS/kotonehara/internal/service/httpclient"
)

var startTime time.Time

func init() {
	startTime = time.Now()
	commands.Register(&commands.Command{
		Name:     "ceksystem",
		As:       []string{"system", "sys"},
		Tags:     "diagnostics",
		IsPrefix: true,
		Exec:     cekSystem,
	})

	commands.Register(&commands.Command{
		Name:     "cekapi",
		As:       []string{"api", "apistatus"},
		Tags:     "diagnostics",
		IsPrefix: true,
		Exec:     cekAPI,
	})
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour

	hours := d / time.Hour
	d -= hours * time.Hour

	minutes := d / time.Minute
	d -= minutes * time.Minute

	seconds := d / time.Second

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dhari", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%djam", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dmenit", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ddetik", seconds))
	}

	return strings.Join(parts, " ")
}

func cekSystem(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	var mStats runtime.MemStats
	runtime.ReadMemStats(&mStats)

	// Convert bytes to MB
	allocMB := float64(mStats.Alloc) / 1024 / 1024
	totalAllocMB := float64(mStats.TotalAlloc) / 1024 / 1024
	sysMB := float64(mStats.Sys) / 1024 / 1024

	uptime := time.Since(startTime)

	response := fmt.Sprintf(`*乂 INFORMATION SYSTEM* 

*• OS:* %s
*• Arch:* %s
*• Compiler:* %s
*• Goroutines:* %d
*• CPU Cores:* %d

*乂 MEMORY USAGE* 

*• Alloc:* %.2f MB
*• Total Alloc:* %.2f MB
*• Sys:* %.2f MB
*• NumGC:* %d

*乂 BOT UPTIME* 

*• Active Time:* %s`,
		runtime.GOOS,
		runtime.GOARCH,
		runtime.Version(),
		runtime.NumGoroutine(),
		runtime.NumCPU(),
		allocMB,
		totalAllocMB,
		sysMB,
		mStats.NumGC,
		formatDuration(uptime),
	)

	m.Reply(ctx, response)
}

func cekAPI(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	endpoints := []apiEndpoint{
		{Name: "BASE API", URL: cfg.BASEApiURL},
		{Name: "BASE S3", URL: cfg.BASES3URL},
		{Name: "RemoveBG", URL: cfg.RemoveBGURL},
		{Name: "Quote API", URL: cfg.QuoteAPIURL},
	}

	httpClient := httpclient.New("", 5*time.Second).HTTP
	results := []string{"*乂 API CONNECTIVITY CHECK*", ""}
	for _, endpoint := range endpoints {
		results = append(results, checkAPIEndpoint(ctx, httpClient, endpoint))
	}

	m.Reply(ctx, strings.Join(results, "\n"))
}

type apiEndpoint struct {
	Name        string
	URL         string
	ConfigError string
}

func checkAPIEndpoint(ctx context.Context, httpClient *http.Client, endpoint apiEndpoint) string {
	if endpoint.ConfigError != "" {
		return fmt.Sprintf("• *%s:* [🔴 Konfigurasi tidak valid]", endpoint.URL)
	}
	if endpoint.URL == "" {
		return fmt.Sprintf("• *%s:* [⚠️ Belum dikonfigurasi]", endpoint.Name)
	}

	target := strings.TrimRight(endpoint.URL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return fmt.Sprintf("• *%s:* [🔴 URL tidak valid]", endpoint.URL)
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return fmt.Sprintf("• *%s:* [🔴 Tidak terjangkau]", endpoint.URL)
	}
	resp.Body.Close()

	if resp.StatusCode < http.StatusInternalServerError {
		return fmt.Sprintf("• *%s:* [🟢 Terjangkau] (%dms)", endpoint.URL, elapsed)
	}
	return fmt.Sprintf("• *%s:* [🟡 HTTP %d] (%dms)", endpoint.Name, resp.StatusCode, elapsed)
}
