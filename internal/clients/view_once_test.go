package clients

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type rvoTransportStub struct {
	downloadErr, uploadErr, sendErr error
	downloads, uploads, sends       int
	file                            string
	kind                            whatsmeow.MediaType
	out                             *waE2E.Message
}

func (s *rvoTransportStub) DownloadToFile(_ context.Context, _ whatsmeow.DownloadableMessage, file whatsmeow.File) error {
	s.downloads++
	s.file = file.(*viewOnceFile).File.Name()
	if s.downloadErr != nil {
		return s.downloadErr
	}
	_, err := file.Write([]byte("media bytes"))
	return err
}
func (s *rvoTransportStub) UploadReader(_ context.Context, r io.Reader, _ io.ReadWriteSeeker, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	s.uploads++
	s.kind = kind
	data, err := io.ReadAll(r)
	if err != nil {
		return whatsmeow.UploadResponse{}, err
	}
	if string(data) != "media bytes" {
		return whatsmeow.UploadResponse{}, errors.New("upload did not read from beginning")
	}
	return whatsmeow.UploadResponse{URL: "https://media/new", DirectPath: "/new", MediaKey: bytes.Repeat([]byte{9}, 32), FileSHA256: bytes.Repeat([]byte{8}, 32), FileEncSHA256: bytes.Repeat([]byte{7}, 32), FileLength: uint64(len(data))}, s.uploadErr
}
func (s *rvoTransportStub) SendMessage(_ context.Context, _ types.JID, out *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	s.sends++
	s.out = out
	return whatsmeow.SendResponse{ID: "sent"}, s.sendErr
}

func rvoSource(kind string) *waE2E.Message {
	key, hash := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	context := &waE2E.ContextInfo{StanzaID: proto.String("old-context")}
	switch kind {
	case "image":
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/old"), MediaKey: key, FileSHA256: hash, FileEncSHA256: hash, ViewOnce: proto.Bool(true), Caption: proto.String("caption"), Mimetype: proto.String("image/jpeg"), Width: proto.Uint32(320), JPEGThumbnail: []byte("thumb"), ContextInfo: context}}
	case "video":
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{DirectPath: proto.String("/old"), MediaKey: key, FileSHA256: hash, FileEncSHA256: hash, ViewOnce: proto.Bool(true), Caption: proto.String("caption"), Mimetype: proto.String("video/mp4"), Seconds: proto.Uint32(5), ContextInfo: context}}
	default:
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{DirectPath: proto.String("/old"), MediaKey: key, FileSHA256: hash, FileEncSHA256: hash, ViewOnce: proto.Bool(true), Mimetype: proto.String("audio/ogg; codecs=opus"), Seconds: proto.Uint32(5), PTT: proto.Bool(true), Waveform: []byte{1, 2}, ContextInfo: context}}
	}
}

func TestRVOStreamsAndSendsOrdinaryMediaWithoutMutatingOriginal(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			source := &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: rvoSource(kind)}}
			before := proto.Clone(source)
			quote := &waE2E.ContextInfo{StanzaID: proto.String("command")}
			transport := &rvoTransportStub{}
			if _, err := sendViewOnceAsNormal(context.Background(), transport, types.NewJID("123", types.DefaultUserServer), source, quote); err != nil {
				t.Fatal(err)
			}
			if transport.downloads != 1 || transport.uploads != 1 || transport.sends != 1 {
				t.Fatal("unexpected transport calls")
			}
			if !proto.Equal(source, before) {
				t.Fatal("source/store message mutated")
			}
			if message.IsViewOnceMessage(transport.out) || transport.out.GetViewOnceMessageV2() != nil {
				t.Fatal("result is still view-once")
			}
			if _, err := os.Stat(transport.file); !os.IsNotExist(err) {
				t.Fatalf("temporary file remains: %v", err)
			}
			var media whatsmeow.DownloadableMessage
			switch kind {
			case "image":
				m := transport.out.GetImageMessage()
				media = m
				if transport.kind != whatsmeow.MediaImage || m.GetCaption() != "caption" || m.GetWidth() != 320 || string(m.GetJPEGThumbnail()) != "thumb" || m.GetContextInfo() != quote {
					t.Fatal("image metadata missing")
				}
			case "video":
				m := transport.out.GetVideoMessage()
				media = m
				if transport.kind != whatsmeow.MediaVideo || m.GetCaption() != "caption" || m.GetSeconds() != 5 || m.GetContextInfo() != quote {
					t.Fatal("video metadata missing")
				}
			case "audio":
				m := transport.out.GetAudioMessage()
				media = m
				if transport.kind != whatsmeow.MediaAudio || !m.GetPTT() || m.GetSeconds() != 5 || len(m.GetWaveform()) != 2 || m.GetContextInfo() != quote {
					t.Fatal("voice note metadata missing")
				}
			}
			if media.GetDirectPath() != "/new" || !bytes.Equal(media.GetMediaKey(), bytes.Repeat([]byte{9}, 32)) {
				t.Fatal("stale upload metadata")
			}
		})
	}
}

func TestRVORejectsOversizedAndUnavailableMediaBeforeDownload(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  *waE2E.Message
		want error
	}{
		{"nil", nil, ErrViewOnceUnavailable},
		{"missing metadata", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, ErrViewOnceUnavailable},
		{"too large", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{FileLength: proto.Uint64(MaxViewOnceBytes + 1)}}, ErrViewOnceTooLarge},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}}, ErrViewOnceUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &rvoTransportStub{}
			_, err := sendViewOnceAsNormal(context.Background(), transport, types.EmptyJID, tc.raw, nil)
			if !errors.Is(err, tc.want) || transport.downloads != 0 || transport.sends != 0 {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

func TestRVOFailureStopsPipelineAndCleansFile(t *testing.T) {
	failure := errors.New("upstream failed")
	for _, stage := range []string{"download", "upload", "send"} {
		t.Run(stage, func(t *testing.T) {
			transport := &rvoTransportStub{}
			switch stage {
			case "download":
				transport.downloadErr = failure
			case "upload":
				transport.uploadErr = failure
			case "send":
				transport.sendErr = failure
			}
			_, err := sendViewOnceAsNormal(context.Background(), transport, types.EmptyJID, rvoSource("image"), nil)
			if !errors.Is(err, failure) {
				t.Fatalf("error lost: %v", err)
			}
			if stage == "download" && transport.uploads != 0 || stage != "send" && transport.sends != 0 {
				t.Fatal("pipeline continued after error")
			}
			if _, err := os.Stat(transport.file); !os.IsNotExist(err) {
				t.Fatalf("file remains on failure: %v", err)
			}
		})
	}
}

func TestRVOFileLimitCannotBeBypassedByCopyWriteAtOrTruncate(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "limited")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	bounded := &viewOnceFile{File: file, max: 5}
	// A TeeReader has no WriterTo; io.Copy must use the wrapper's ReadFrom.
	_, err = io.Copy(bounded, io.TeeReader(strings.NewReader("too many bytes"), io.Discard))
	if !errors.Is(err, ErrViewOnceTooLarge) {
		t.Fatalf("copy bypassed limit: %v", err)
	}
	if _, err := bounded.WriteAt([]byte("xx"), 4); !errors.Is(err, ErrViewOnceTooLarge) {
		t.Fatalf("WriteAt bypassed limit: %v", err)
	}
	if err := bounded.Truncate(6); !errors.Is(err, ErrViewOnceTooLarge) {
		t.Fatalf("Truncate bypassed limit: %v", err)
	}
	if info, err := file.Stat(); err != nil || info.Size() > 5 {
		t.Fatalf("file exceeded cap: %+v %v", info, err)
	}
}

func TestRVOWaitForSlotRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := &rvoTransportStub{}
	if _, err := sendViewOnceAsNormal(ctx, transport, types.EmptyJID, rvoSource("image"), nil); !errors.Is(err, context.Canceled) || transport.downloads != 0 {
		t.Fatalf("cancellation ignored with free slots: %v", err)
	}
	viewOnceSlots <- struct{}{}
	viewOnceSlots <- struct{}{}
	defer func() { <-viewOnceSlots; <-viewOnceSlots }()
	if _, err := sendViewOnceAsNormal(ctx, transport, types.EmptyJID, rvoSource("image"), nil); !errors.Is(err, context.Canceled) || transport.downloads != 0 {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
