package telegram

type StickerSetResponse struct {
	OK          bool        `json:"ok"`
	Description string      `json:"description,omitempty"`
	ErrorCode   int         `json:"error_code,omitempty"`
	Result      *StickerSet `json:"result,omitempty"`
}

type StickerSet struct {
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	StickerType string     `json:"sticker_type"`
	IsAnimated  bool       `json:"is_animated"`
	IsVideo     bool       `json:"is_video"`
	Stickers    []Sticker  `json:"stickers"`
	Thumbnail   *PhotoSize `json:"thumbnail,omitempty"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

type Sticker struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	IsAnimated   bool       `json:"is_animated"`
	IsVideo      bool       `json:"is_video"`
	Type         string     `json:"type"`
	Emoji        string     `json:"emoji,omitempty"`
	SetName      string     `json:"set_name,omitempty"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	Thumb        *PhotoSize `json:"thumb,omitempty"`
	FileSize     int        `json:"file_size,omitempty"`
}

type GetFileResponse struct {
	OK          bool      `json:"ok"`
	Description string    `json:"description,omitempty"`
	ErrorCode   int       `json:"error_code,omitempty"`
	Result      *FileInfo `json:"result,omitempty"`
}

type FileInfo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int    `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}
