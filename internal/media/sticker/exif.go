package sticker

import (
	"encoding/json"
)

type stickerPack struct {
	StickerPackID        string `json:"sticker-pack-id"`
	StickerPackName      string `json:"sticker-pack-name"`
	StickerPackPublisher string `json:"sticker-pack-publisher"`
	AndroidAppStoreLink  string `json:"android-app-store-link"`
	IOSAppStoreLink      string `json:"ios-app-store-link"`
}

const (
	packID     = "com.snowcorp.stickerly.android.stickercontentprovider b5e7275f-f1de-4137-961f-57becfad34f2"
	playStore  = "https://play.google.com/store/apps/details?id=com.pubg.newstate&hl=in&gl=US"
	appleStore = "https://apps.apple.com/us/app/pubg-mobile-3rd-anniversary/id1330123889"
)

func buildExif(author string) []byte {
	jb, _ := json.Marshal(stickerPack{
		StickerPackID:        packID,
		StickerPackName:      "Kotone Oohara",
		StickerPackPublisher: author,
		AndroidAppStoreLink:  playStore,
		IOSAppStoreLink:      appleStore,
	})
	l := len(jb)
	head := []byte{0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x41, 0x57, 0x07, 0x00}
	var lenByte byte
	var code []byte
	if l > 256 {
		lenByte = byte(l - 256)
		code = []byte{0x01, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00}
	} else {
		lenByte = byte(l)
		code = []byte{0x00, 0x00, 0x16, 0x00, 0x00, 0x00}
	}
	out := make([]byte, 0, len(head)+1+len(code)+len(jb))
	out = append(out, head...)
	out = append(out, lenByte)
	out = append(out, code...)
	out = append(out, jb...)
	return out
}
