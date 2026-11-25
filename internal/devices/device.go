package devices

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/handlers"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	meowcaller "github.com/purpshell/meowcaller"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waWa6"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

const (
	defaultLIDWaitTimeout  = 30 * time.Second
	defaultLIDPollInterval = 250 * time.Millisecond
)

var clientPayloadMu sync.Mutex

func applyClientBrowserProfile(isAndroid bool) {
	clientPayloadMu.Lock()
	defer clientPayloadMu.Unlock()
	applyClientBrowserProfileLocked(isAndroid)
}

func applyClientBrowserProfileLocked(isAndroid bool) {
	if isAndroid {
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
		store.SetOSInfo("Android 13", [3]uint32{13, 0, 0})
		store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_WEB.Enum()
		store.BaseClientPayload.UserAgent.Device = proto.String("Android 13")
		return
	}
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	store.SetOSInfo("Windows", [3]uint32{10, 0, 0})
	store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_WEB.Enum()
	store.BaseClientPayload.UserAgent.Device = proto.String("Desktop")
}

func hasDeviceLID(dev *store.Device) bool {
	if dev == nil || dev.ID == nil || dev.ID.IsEmpty() {
		return false
	}
	lid := dev.GetLID()
	return !lid.IsEmpty() && lid.Server == types.HiddenUserServer
}

type clientIdentityManager struct {
	dev           *store.Device
	androidActive atomic.Bool
	switching     atomic.Bool
	restartConn   func()
	log           waLog.Logger
	pollInterval  time.Duration
	waitTimeout   time.Duration
}

func newClientIdentityManager(dev *store.Device, log waLog.Logger, restartConn func()) *clientIdentityManager {
	m := &clientIdentityManager{
		dev:          dev,
		restartConn:  restartConn,
		log:          log,
		pollInterval: defaultLIDPollInterval,
		waitTimeout:  defaultLIDWaitTimeout,
	}
	hasLID := hasDeviceLID(dev)
	m.androidActive.Store(hasLID)
	applyClientBrowserProfile(hasLID)
	return m
}

func (m *clientIdentityManager) payload() *waWa6.ClientPayload {
	if m == nil || m.dev == nil {
		return nil
	}
	clientPayloadMu.Lock()
	defer clientPayloadMu.Unlock()
	applyClientBrowserProfileLocked(m.androidActive.Load())
	return m.dev.GetClientPayload()
}

func (m *clientIdentityManager) resolveLID(ctx context.Context) types.JID {
	if m.dev == nil || m.dev.ID == nil || m.dev.ID.IsEmpty() {
		return types.EmptyJID
	}
	if lid := m.dev.GetLID(); !lid.IsEmpty() && lid.Server == types.HiddenUserServer {
		return lid
	}
	if m.dev.LIDs != nil {
		if lid, err := m.dev.LIDs.GetLIDForPN(ctx, m.dev.GetJID()); err == nil && !lid.IsEmpty() && lid.Server == types.HiddenUserServer {
			return lid
		}
	}
	return types.EmptyJID
}

func (m *clientIdentityManager) onConnected(ctx context.Context) {
	if m == nil || m.androidActive.Load() {
		return
	}
	if !m.switching.CompareAndSwap(false, true) {
		return
	}
	go m.waitForLIDAndRestart(ctx)
}

func (m *clientIdentityManager) waitForLIDAndRestart(ctx context.Context) {
	defer m.switching.Store(false)

	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, m.waitTimeout)
	defer cancel()

	interval := m.pollInterval
	if interval <= 0 {
		interval = defaultLIDPollInterval
	}

	var lid types.JID
	for {
		lid = m.resolveLID(waitCtx)
		if !lid.IsEmpty() {
			break
		}
		timer := time.NewTimer(interval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if m.log != nil && ctx.Err() == nil {
				m.log.Warnf("timed out waiting for @lid before switching profile to Android 13")
			}
			return
		case <-timer.C:
		}
	}

	if !m.androidActive.CompareAndSwap(false, true) {
		return
	}
	applyClientBrowserProfile(true)
	if m.log != nil {
		m.log.Infof("logged in with LID %s; switching profile to Android 13 and restarting connection", lid)
	}
	if m.restartConn != nil {
		m.restartConn()
	}
}

func (m *clientIdentityManager) onLoggedOut() {
	if m == nil {
		return
	}
	m.androidActive.Store(false)
	applyClientBrowserProfile(false)
}

const (
	// maxConcurrentEvents bounds handlers running at once (downloads, ffmpeg, ...).
	maxConcurrentEvents = 20
	// maxQueuedEvents bounds handlers accepted but not finished, so a burst
	// waits for a free slot instead of being silently dropped, while a flood
	// still cannot grow memory without limit.
	maxQueuedEvents = 500
)

func New(c *sqlstore.Container, cfg config.Config, ctx context.Context) *Devices {
	if ctx == nil {
		ctx = context.Background()
	}
	commands.SetCooldown(cfg.Cooldown)

	d := &Devices{
		container: c,
		log:       waLog.Stdout("devices", "INFO", true),
		timeout:   10 * time.Second,
		ctx:       ctx,
		cfg:       cfg,
		eventSem:  make(chan struct{}, maxConcurrentEvents),
		maxQueued: maxQueuedEvents,
		stopCh:    make(chan struct{}),
		drained:   make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			d.beginShutdown()
		case <-d.drained:
		}
	}()
	return d
}

// Shutdown stops accepting command events and waits for active commands to finish.
// The wait is bounded by ctx, while active commands continue using the root context.
func (d *Devices) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d.beginShutdown()
	select {
	case <-d.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Devices) beginShutdown() {
	d.stopOnce.Do(func() {
		d.lifecycleMu.Lock()
		d.stopping = true
		close(d.stopCh)
		d.lifecycleMu.Unlock()
		go func() {
			d.active.Wait()
			close(d.drained)
		}()
	})
}

// tryStartCommand accepts run for asynchronous execution. It never blocks the
// whatsmeow event loop: accepted work waits in its own goroutine for one of the
// maxConcurrentEvents slots. It returns false (and logs) only when shutting
// down or when maxQueued handlers are already pending.
func (d *Devices) tryStartCommand(run func()) bool {
	if run == nil {
		return false
	}

	d.lifecycleMu.Lock()
	if d.stopping || d.ctx.Err() != nil {
		d.lifecycleMu.Unlock()
		return false
	}
	if d.queued >= d.maxQueued {
		d.lifecycleMu.Unlock()
		d.log.Warnf("event queue full (%d pending), dropping message event", d.maxQueued)
		return false
	}
	d.queued++
	d.active.Add(1)
	d.lifecycleMu.Unlock()

	go func() {
		defer func() {
			d.lifecycleMu.Lock()
			d.queued--
			d.lifecycleMu.Unlock()
			d.active.Done()
		}()

		select {
		case d.eventSem <- struct{}{}:
		case <-d.stopCh:
			return
		case <-d.ctx.Done():
			return
		}
		defer func() { <-d.eventSem }()

		// A slot may free up at the same moment shutdown begins; queued work
		// that has not started yet is dropped rather than run during drain.
		select {
		case <-d.stopCh:
			return
		default:
		}
		run()
	}()
	return true
}

func (d *Devices) GetDefaultDevice(ctx context.Context) (*store.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	dev, err := d.container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first device: %w", err)
	}
	if dev == nil {
		return nil, errors.New("no device found (not paired yet)")
	}
	return dev, nil
}

func (d *Devices) NewClient(dev *store.Device) *whatsmeow.Client {
	clientLog := waLog.Stdout("whatsmeow", "ERROR", true)

	if d.cfg.DisableContactImport {
		d.log.Infof("contact import disabled")
		disableContactImportForDevice(dev)
	}

	client := whatsmeow.NewClient(dev, clientLog)
	identity := newClientIdentityManager(dev, d.log, client.ResetConnection)
	client.GetClientPayload = identity.payload
	callClient := meowcaller.NewClient(client)
	callClient.OnIncomingCall(func(call *meowcaller.Call) {
		d.log.Infof("incoming call from %s (%s)", call.Peer().String(), call.ID())
	})
	if d.cfg.DisableContactImport {
		disableContactImportForDevice(client.Store)
	}
	client.AddEventHandler(d.registerEventHandler(client, callClient, identity))
	return client
}

func (d *Devices) registerEventHandler(client *whatsmeow.Client, callClient *meowcaller.Client, identity *clientIdentityManager) func(evt interface{}) {
	c := clients.New(client, d.cfg, callClient)
	m := message.NewParser(c, d.cfg)
	m.ResolveIdentity = c.ResolveIdentity
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			if d.ctx.Err() != nil {
				return
			}
			c.RememberAlbumChild(v)
			parse := m.Parse(d.ctx, v)

			d.tryStartCommand(func() {
				defer func() {
					if r := recover(); r != nil {
						d.log.Errorf("command panic: %v", r)
					}
				}()

				handlers.CheckAFK(d.ctx, c, parse, d.cfg)

				if !commands.CanHandle(parse.Body, d.cfg) {
					return
				}

				commands.CommandExec(d.ctx, c, parse, d.cfg)
			})

		case *events.Connected:
			if d.cfg.DisableContactImport {
				disableContactImportForDevice(client.Store)
			}
			d.log.Infof("connected")
			identity.onConnected(d.ctx)
		case *events.LoggedOut:
			identity.onLoggedOut()
		case *events.Disconnected:
			d.log.Warnf("disconnected")
		}
	}
}
