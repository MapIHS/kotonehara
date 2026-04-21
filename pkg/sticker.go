package pkg

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/media/sticker"
	"github.com/MapIHS/kotonehara/internal/message"
	"go.mau.fi/whatsmeow"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

func stc(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	if m.Media == nil {
		m.Reply(ctx, "Kirim atau balas gambar dulu, yaa.")
		return
	}

	raw, err := client.WA.Download(ctx, m.Media)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	stc, err := sticker.BuildSticker(ctx, raw, m.PushName, m.IsQuotedSticker, m.IsVideo || m.IsQuotedVideo)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Ada yang salah: %s", err))
	}

	if _, err := client.SendSticker(ctx, m.From, stc, false, false, nil); err != nil {
		m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		return
	}
}

type ZipFileEntry struct {
	Name string
	Data []byte
}

func extractZipFiles(zipData []byte) ([]ZipFileEntry, error) {
	reader := bytes.NewReader(zipData)
	zr, err := zip.NewReader(reader, int64(len(zipData)))
	if err != nil {
		return nil, err
	}

	var entries []ZipFileEntry
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		f, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		entries = append(entries, ZipFileEntry{
			Name: file.Name,
			Data: data,
		})
	}
	return entries, nil
}

func repackageZip(entries []ZipFileEntry) ([]byte, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	for _, entry := range entries {
		f, err := w.Create(entry.Name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(entry.Data); err != nil {
			return nil, err
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getFloat(v interface{}, fallback float64) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f
		}
	}
	return fallback
}

func slottie(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	imgMsg := m.Message.GetImageMessage()
	if imgMsg == nil {
		m.Reply(ctx, "Kirim gambar lalu reply sticker lottie-nya, yaa.\n\nList Animasi yang tersedia:\n1. rotate\n2. shake\n3. blink")
		return
	}

	fmt.Println(m.QuotedMsg)

	if m.QuotedMsg == nil {
		m.Reply(ctx, "Reply sticker lottie-nya dulu, yaa.")
		return
	}

	imgRaw, err := client.WA.Download(ctx, imgMsg)
	if err != nil || len(imgRaw) == 0 {
		m.Reply(ctx, "Gambarnya belum bisa diambil, yaa.")
		return
	}

	quoted := m.QuotedMsg
	var stickerMsg whatsmeow.DownloadableMessage
	if msg := quoted.GetLottieStickerMessage().GetMessage().GetStickerMessage(); msg != nil {
		stickerMsg = msg
	} else if msg := quoted.GetStickerMessage(); msg != nil {
		stickerMsg = msg
	}
	if stickerMsg == nil {
		m.Reply(ctx, "Sticker lottie tidak ditemukan di pesan yang direply, yaa.")
		return
	}

	raw, err := client.WA.Download(ctx, stickerMsg)
	if err != nil || len(raw) == 0 {
		m.Reply(ctx, "File zip belum bisa diambil, yaa.")
		return
	}

	entries, err := extractZipFiles(raw)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Gagal ekstrak zip: %s", err))
		return
	}

	var secondaryIdx int = -1
	for i, entry := range entries {
		if filepath.Base(entry.Name) == "animation_secondary.json" {
			secondaryIdx = i

			break
		}
	}

	if secondaryIdx == -1 {
		m.Reply(ctx, "animation_secondary.json tidak ditemukan")
		return
	}

	var secondaryData map[string]interface{}
	if err := json.Unmarshal(entries[secondaryIdx].Data, &secondaryData); err != nil {
		m.Reply(ctx, fmt.Sprintf("File lottie tidak valid: %s", err))
		return
	}

	mime := http.DetectContentType(imgRaw)
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	dataURI := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(imgRaw))

	imgCfg, _, err := image.DecodeConfig(bytes.NewReader(imgRaw))
	if err != nil {
		imgCfg.Width = int(getFloat(secondaryData["w"], 512))
		imgCfg.Height = int(getFloat(secondaryData["h"], 512))
	}
	imgW := float64(imgCfg.Width)
	imgH := float64(imgCfg.Height)
	if imgW == 0 || imgH == 0 {
		imgW = getFloat(secondaryData["w"], 512)
		imgH = getFloat(secondaryData["h"], 512)
	}

	assets, _ := secondaryData["assets"].([]interface{})
	if assets == nil {
		assets = []interface{}{}
	}

	imageAssetID := "image_swap"
	assetUpdated := false
	for i, asset := range assets {
		m, ok := asset.(map[string]interface{})
		if !ok {
			continue
		}
		if p, ok := m["p"].(string); ok && p != "" {
			if id, ok := m["id"].(string); ok && id != "" {
				imageAssetID = id
			}
			m["w"] = imgW
			m["h"] = imgH
			m["u"] = ""
			m["p"] = dataURI
			m["e"] = float64(1)
			assets[i] = m
			assetUpdated = true
			break
		}
	}
	if !assetUpdated {
		assets = append(assets, map[string]interface{}{
			"id": imageAssetID,
			"w":  imgW,
			"h":  imgH,
			"u":  "",
			"p":  dataURI,
			"e":  float64(1),
		})
	}
	secondaryData["assets"] = assets

	compW := getFloat(secondaryData["w"], imgW)
	compH := getFloat(secondaryData["h"], imgH)
	originalOp := getFloat(secondaryData["op"], 60)
	compOp := originalOp
	compFr := getFloat(secondaryData["fr"], 30)
	minFrames := compFr * 10
	if compOp < minFrames {
		compOp = minFrames
		secondaryData["op"] = compOp
	}
	if compW == 0 {
		compW = imgW
	}
	if compH == 0 {
		compH = imgH
	}

	layers, _ := secondaryData["layers"].([]interface{})
	if layers == nil {
		layers = []interface{}{}
	}
	maxInd := 0
	layerIdx := -1
	for i, layer := range layers {
		lm, ok := layer.(map[string]interface{})
		if !ok {
			continue
		}
		if compOp > originalOp {
			if op, ok := lm["op"].(float64); ok && op == originalOp {
				lm["op"] = compOp
			}
		}
		if ind, ok := lm["ind"].(float64); ok && int(ind) > maxInd {
			maxInd = int(ind)
		}
		if nm, ok := lm["nm"].(string); ok && nm == "image_swap" {
			layerIdx = i
		}
		if ref, ok := lm["refId"].(string); ok && ref == imageAssetID {
			layerIdx = i
		}
	}

	scale := 100.0
	if imgW > 0 && imgH > 0 && compW > 0 && compH > 0 {
		scale = math.Min(compW/imgW, compH/imgH) * 100
	}
	posKey := map[string]interface{}{"a": float64(0), "k": []interface{}{compW / 2, compH / 2, 0}}
	scaleKey := map[string]interface{}{"a": float64(0), "k": []interface{}{scale, scale, 100}}
	rotKey := map[string]interface{}{"a": float64(0), "k": float64(0)}
	opKey := map[string]interface{}{"a": float64(0), "k": float64(100)}

	animType := strings.ToLower(strings.TrimSpace(m.Query))

	if compOp > 1 {
		tMid := math.Floor(compOp / 2)
		if tMid < 1 {
			tMid = 1
		}

		switch animType {
		case "putar", "rotate":
			rotKey = map[string]interface{}{
				"a": float64(1),
				"k": []interface{}{
					map[string]interface{}{
						"t": float64(0),
						"s": []interface{}{0},
						"e": []interface{}{360},
					},
					map[string]interface{}{
						"t": compOp,
						"s": []interface{}{360},
					},
				},
			}
		case "kedip", "blink":
			opKey = map[string]interface{}{
				"a": float64(1),
				"k": []interface{}{
					map[string]interface{}{"t": float64(0), "s": []interface{}{100}, "e": []interface{}{0}},
					map[string]interface{}{"t": float64(compOp / 4), "s": []interface{}{0}, "e": []interface{}{100}},
					map[string]interface{}{"t": float64(compOp / 2), "s": []interface{}{100}, "e": []interface{}{0}},
					map[string]interface{}{"t": float64(compOp * 3 / 4), "s": []interface{}{0}, "e": []interface{}{100}},
					map[string]interface{}{"t": compOp, "s": []interface{}{100}},
				},
			}
		case "goyang", "shake":
			rotKey = map[string]interface{}{
				"a": float64(1),
				"k": []interface{}{
					map[string]interface{}{"t": float64(0), "s": []interface{}{0}, "e": []interface{}{15}},
					map[string]interface{}{"t": float64(compOp / 4), "s": []interface{}{15}, "e": []interface{}{-15}},
					map[string]interface{}{"t": float64(compOp * 3 / 4), "s": []interface{}{-15}, "e": []interface{}{0}},
					map[string]interface{}{"t": compOp, "s": []interface{}{0}},
				},
			}
		default:
			amp := math.Min(compW, compH) * 0.08
			if amp < 20 {
				amp = 20
			}
			y0 := compH / 2
			y1 := y0 - amp
			posKey = map[string]interface{}{
				"a": float64(1),
				"k": []interface{}{
					map[string]interface{}{
						"t": float64(0),
						"s": []interface{}{compW / 2, y0, 0},
						"e": []interface{}{compW / 2, y1, 0},
						"i": map[string]interface{}{"x": 0.42, "y": 0},
						"o": map[string]interface{}{"x": 0.58, "y": 1},
					},
					map[string]interface{}{
						"t": tMid,
						"s": []interface{}{compW / 2, y1, 0},
						"e": []interface{}{compW / 2, y0, 0},
						"i": map[string]interface{}{"x": 0.42, "y": 0},
						"o": map[string]interface{}{"x": 0.58, "y": 1},
					},
					map[string]interface{}{
						"t": compOp,
						"s": []interface{}{compW / 2, y0, 0},
					},
				},
			}
			scaleKey = map[string]interface{}{
				"a": float64(1),
				"k": []interface{}{
					map[string]interface{}{
						"t": float64(0),
						"s": []interface{}{scale, scale, 100},
						"e": []interface{}{scale * 1.06, scale * 1.06, 100},
						"i": map[string]interface{}{"x": 0.42, "y": 0},
						"o": map[string]interface{}{"x": 0.58, "y": 1},
					},
					map[string]interface{}{
						"t": tMid,
						"s": []interface{}{scale * 1.06, scale * 1.06, 100},
						"e": []interface{}{scale, scale, 100},
						"i": map[string]interface{}{"x": 0.42, "y": 0},
						"o": map[string]interface{}{"x": 0.58, "y": 1},
					},
					map[string]interface{}{
						"t": compOp,
						"s": []interface{}{scale, scale, 100},
					},
				},
			}
		}
	}
	imageLayer := map[string]interface{}{
		"ddd":   0,
		"ind":   float64(maxInd + 1),
		"ty":    2,
		"nm":    "image_swap",
		"refId": imageAssetID,
		"sr":    1,
		"ks": map[string]interface{}{
			"o": opKey,
			"r": rotKey,
			"p": posKey,
			"a": map[string]interface{}{"a": float64(0), "k": []interface{}{imgW / 2, imgH / 2, 0}},
			"s": scaleKey,
		},
		"ao": 0,
		"ip": 0,
		"op": compOp,
		"st": 0,
		"bm": 0,
	}
	if layerIdx >= 0 {
		layers[layerIdx] = imageLayer
	} else {
		layers = append(layers, imageLayer)
	}
	secondaryData["layers"] = layers

	editedJSON, err := json.Marshal(secondaryData)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Gagal encode JSON: %s", err))
		return
	}
	entries[secondaryIdx].Data = editedJSON

	newZip, err := repackageZip(entries)
	if err != nil {
		m.Reply(ctx, fmt.Sprintf("Gagal zip ulang: %s", err))
		return
	}

	if _, err := client.SendSticker(ctx, m.From, newZip, true, true, nil); err != nil {
		m.Reply(ctx, "Stikernya belum bisa dikirim, yaa.")
		fmt.Println(err)
		return
	}
}

func init() {
	commands.Register(&commands.Command{
		Name:     "sticker",
		As:       []string{"s", "stiker"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     stc,
	})
	commands.Register(&commands.Command{
		Name:     "slottie",
		As:       []string{"stickerlottie"},
		Tags:     "convert",
		IsPrefix: true,
		Exec:     slottie,
	})
}
