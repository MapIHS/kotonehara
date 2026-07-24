package pkg

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	meowcaller "github.com/purpshell/meowcaller"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
)

const noAudioCallMaxDuration = 60 * time.Second
const callAudioDownloadTimeout = 60 * time.Second
const callAudioEndDelay = 5 * time.Second

type callAudioMedia struct {
	media    whatsmeow.DownloadableMessage
	mimeType string
}

func callCmd(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	_ = cfg

	if client.Calls == nil {
		_, _ = m.Reply(ctx, "Fitur call belum aktif.")
		return
	}

	args := strings.Fields(m.Query)
	target, audioPath := callTargetAndAudio(m, args)
	if target == "" {
		_, _ = m.Reply(ctx, "Target call belum ada. Balas pesan orangnya, tag, atau masukkan nomornya. Contoh: .call 628xxxx")
		return
	}

	source, cleanupSource, hasAudio, err := openCallAudioSourceForMessage(ctx, client, m, audioPath)
	if err != nil {
		_, _ = m.Reply(ctx, fmt.Sprintf("Audio belum bisa dipakai: %s", err))
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	call, err := client.Calls.Call(callCtx, target)
	if err != nil {
		cleanupSource()
		_, _ = m.Reply(ctx, "Gagal mulai call: "+err.Error())
		return
	}

	callDone := make(chan struct{})
	var doneOnce sync.Once
	finish := func(reason string) {
		doneOnce.Do(func() {
			close(callDone)
			cleanupSource()
			_, _ = m.Reply(ctx, "Call selesai: "+reason)
		})
	}

	call.OnEnd(finish)
	call.OnReady(func() {
		if source == nil {
			return
		}
		player := call.Play(source)
		player.OnFinish(func() {
			go func() {
				timer := time.NewTimer(callAudioEndDelay)
				defer timer.Stop()
				select {
				case <-callDone:
				case <-ctx.Done():
					_ = call.Hangup()
				case <-timer.C:
					_ = call.Hangup()
				}
			}()
		})
	})

	go func() {
		<-ctx.Done()
		_ = call.Hangup()
	}()

	if !hasAudio {
		go func() {
			timer := time.NewTimer(noAudioCallMaxDuration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
				_ = call.Hangup()
				finish("timeout 60s")
			}
		}()
	}

	msg := fmt.Sprintf("Mulai call ke %s.", target)
	if hasAudio {
		msg += "\nAudio akan diputar saat call tersambung."
		msg += "\nAuto Cancel 5 detik setelah audio selesai."
	} else {
		msg += "\nAuto Cancel setelah 60 detik."
	}
	_, _ = m.Reply(ctx, msg)
}

func callTargetAndAudio(m *message.Message, args []string) (target, audioPath string) {
	if m.QuotedMsg != nil {
		if ext := m.Message.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil {
			if participant := strings.TrimSpace(ext.GetContextInfo().GetParticipant()); participant != "" {
				if len(args) > 0 && !isCallAudioPath(args[0]) {
					if len(args) > 1 {
						audioPath = args[1]
					}
					return args[0], audioPath
				}
				if len(args) > 0 {
					audioPath = args[0]
				}
				return participant, audioPath
			}
		}
		if !m.IsGroup && !m.From.IsEmpty() {
			if len(args) > 0 && !isCallAudioPath(args[0]) {
				if len(args) > 1 {
					audioPath = args[1]
				}
				return args[0], audioPath
			}
			if len(args) > 0 {
				audioPath = args[0]
			}
			return m.From.String(), audioPath
		}
	}

	if m.ID != nil && len(m.ID.MentionedJID) > 0 {
		if len(args) > 1 {
			audioPath = args[1]
		}
		return strings.TrimSpace(m.ID.MentionedJID[0]), audioPath
	}

	if len(args) == 0 {
		return "", ""
	}
	if len(args) > 1 {
		audioPath = args[1]
	}
	return args[0], audioPath
}

func openCallAudioSourceForMessage(ctx context.Context, client *clients.Client, m *message.Message, audioPath string) (meowcaller.AudioSource, func(), bool, error) {
	if audioPath != "" {
		source, err := openCallAudioSource(audioPath)
		if err != nil {
			return nil, func() {}, false, err
		}
		return source, func() { _ = source.Close() }, true, nil
	}

	media, ok := pickCallAudioMedia(m)
	if !ok {
		return nil, func() {}, false, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, callAudioDownloadTimeout)
	defer cancel()

	data, err := client.WA.Download(opCtx, media.media)
	if err != nil {
		return nil, func() {}, false, err
	}
	if len(data) == 0 {
		return nil, func() {}, false, fmt.Errorf("audio kosong")
	}

	tmp, err := os.CreateTemp("", "kotonehara-call-*"+callAudioExt(media.mimeType, data))
	if err != nil {
		return nil, func() {}, false, err
	}
	path := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return nil, func() {}, false, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return nil, func() {}, false, err
	}

	source, err := openCallAudioSource(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, func() {}, false, err
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = source.Close()
			_ = os.Remove(path)
		})
	}
	return source, cleanup, true, nil
}

func pickCallAudioMedia(m *message.Message) (callAudioMedia, bool) {
	if m == nil {
		return callAudioMedia{}, false
	}
	if media, ok := callAudioMediaFromMessage(m.QuotedMsg); ok {
		return media, true
	}
	return callAudioMediaFromMessage(m.Message)
}

func callAudioMediaFromMessage(msg *waE2E.Message) (callAudioMedia, bool) {
	if msg == nil {
		return callAudioMedia{}, false
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return callAudioMedia{media: aud, mimeType: aud.GetMimetype()}, true
	}
	if doc := msg.GetDocumentMessage(); doc != nil && strings.HasPrefix(doc.GetMimetype(), "audio/") {
		return callAudioMedia{media: doc, mimeType: doc.GetMimetype()}, true
	}
	return callAudioMedia{}, false
}

func callAudioExt(mimeType string, data []byte) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch mimeType {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	}

	if len(data) >= 4 {
		switch {
		case string(data[:4]) == "OggS":
			return ".ogg"
		case string(data[:4]) == "RIFF":
			return ".wav"
		case len(data) >= 3 && string(data[:3]) == "ID3":
			return ".mp3"
		}
	}

	switch http.DetectContentType(data) {
	case "audio/mpeg":
		return ".mp3"
	case "audio/wave", "audio/wav", "audio/x-wav":
		return ".wav"
	default:
		return ".ogg"
	}
}

func isCallAudioPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".wav", ".opus", ".ogg":
		return true
	default:
		return false
	}
}

func openCallAudioSource(path string) (meowcaller.AudioSource, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return meowcaller.MP3File(path)
	case ".wav":
		return meowcaller.WAVFile(path)
	case ".opus", ".ogg":
		return meowcaller.OpusFile(path)
	default:
		return nil, fmt.Errorf("format %q belum didukung (pakai .mp3, .wav, .ogg, atau .opus)", filepath.Ext(path))
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:        "call",
		Tags:        "owner",
		Description: "Telepon target via WhatsApp call. Opsional play file .mp3/.wav/.opus lokal atau audio yang direply",
		Disable:     true,
		IsPrefix:    true,
		IsOwner:     true,
		Exec:        callCmd,
	})
}
