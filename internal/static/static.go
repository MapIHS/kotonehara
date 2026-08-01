package static

import _ "embed"

//go:embed spam.webp
var SpamSticker []byte

//go:embed siapa.webp
var OwnerSticker []byte

//go:embed QRIS.png
var QRISImage []byte
