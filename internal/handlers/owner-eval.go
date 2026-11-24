package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/unrestricted"
)

const (
	ownerEvalTimeout       = 10 * time.Second
	ownerEvalMaxQuerySize  = 64 * 1024
	ownerEvalMaxOutputSize = 256 * 1024
	ownerEvalMaxReplySize  = 65536
)

// Eval remains a trusted-owner administration tool, not an untrusted-code
// sandbox. Bound concurrency and buffers so accidental output cannot grow RAM.
var ownerEvalSlot = make(chan struct{}, 1)

type limitedBuffer struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{remaining: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if b.remaining <= 0 {
		b.truncated = true
		return original, nil
	}
	if len(p) > b.remaining {
		p = p[:b.remaining]
		b.truncated = true
	}
	n, err := b.buf.Write(p)
	b.remaining -= n
	return original, err
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

func ownerEval(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	query := strings.TrimSpace(m.Query)
	if body := strings.TrimSpace(m.Body); body != "" {
		for _, prefix := range []string{"=>", ">"} {
			if strings.HasPrefix(body, prefix) {
				query = strings.TrimSpace(strings.TrimPrefix(body, prefix))
				break
			}
		}
	}
	if query == "" {
		_, _ = m.Reply(ctx, "Kode Go dibutuhkan, yaa.\n\nContoh:\n> 1 + 1\n> time.Now().Format(time.RFC3339)\n> strings.ToUpper(\"halo\")\n> M.Sender.User")
		return
	}
	if len(query) > ownerEvalMaxQuerySize {
		_, _ = m.Reply(ctx, "Kode terlalu panjang. Maksimum 64 KiB, yaa.")
		return
	}

	if pretty, ok := tryPrettyJSON(query); ok {
		_, _ = m.Reply(ctx, pretty)
		return
	}

	select {
	case ownerEvalSlot <- struct{}{}:
		defer func() { <-ownerEvalSlot }()
	case <-ctx.Done():
		return
	default:
		_, _ = m.Reply(ctx, "Eval lain masih berjalan. Tunggu sampai selesai, yaa.")
		return
	}

	stdout := newLimitedBuffer(ownerEvalMaxOutputSize)
	stderr := newLimitedBuffer(ownerEvalMaxOutputSize)
	i := interp.New(interp.Options{
		Stdout:       stdout,
		Stderr:       stderr,
		Unrestricted: true,
	})

	// Full host access is intentional for trusted-owner debugging. Do not
	// present this interpreter as a sandbox or accept code from other users.
	if err := i.Use(stdlib.Symbols); err != nil {
		_, _ = m.Reply(ctx, "Runtime Go belum bisa disiapkan, yaa.")
		return
	}
	if err := i.Use(unrestricted.Symbols); err != nil {
		_, _ = m.Reply(ctx, "Runtime Go belum bisa disiapkan, yaa.")
		return
	}
	if err := i.Use(interp.Exports{
		"bot/bot": {
			"Client": reflect.ValueOf(client),
			"M":      reflect.ValueOf(m),
			"Ctx":    reflect.ValueOf(ctx),
			"Cfg":    reflect.ValueOf(cfg),
		},
	}); err != nil {
		_, _ = m.Reply(ctx, "Konteks bot belum bisa disiapkan, yaa.")
		return
	}

	if _, err := i.Eval(`import (
		. "bot/bot"
		"context"
		"encoding/json"
		"fmt"
		"io"
		"math"
		"net/http"
		"os"
		"os/exec"
		"regexp"
		"strings"
		"sync"
		"time"
	)`); err != nil {
		_, _ = m.Reply(ctx, "Package runtime Go belum bisa dimuat, yaa.")
		return
	}

	evalCtx, cancel := context.WithTimeout(ctx, ownerEvalTimeout)
	defer cancel()

	res, err := i.EvalWithContext(evalCtx, query)
	if evalCtx.Err() != nil {
		if errors.Is(evalCtx.Err(), context.DeadlineExceeded) {
			_, _ = m.Reply(ctx, "Evaluasi Go timeout (10 detik), yaa.")
		}
		return
	}

	var parts []string
	outText := strings.TrimSpace(stdout.String())
	errText := strings.TrimSpace(stderr.String())
	if stdout.truncated {
		outText = strings.TrimSpace(outText) + "\n... [output dipotong]"
	}
	if stderr.truncated {
		errText = strings.TrimSpace(errText) + "\n... [stderr dipotong]"
	}
	if outText != "" {
		if pretty, ok := tryPrettyJSON(outText); ok {
			outText = pretty
		}
		parts = append(parts, outText)
	}
	if errText != "" {
		parts = append(parts, fmt.Sprintf("Stderr:\n%s", errText))
	}
	if err != nil {
		parts = append(parts, fmt.Sprintf("Error:\n%s", err.Error()))
	} else if res.IsValid() && res.CanInterface() {
		valStr := formatEvalValue(res.Interface())
		if outText == "" && valStr != "" && valStr != "<nil>" {
			parts = append(parts, valStr)
		}
	} else if len(parts) == 0 {
		parts = append(parts, "nil")
	}

	reply := truncateOwnerEvalReply(strings.TrimSpace(strings.Join(parts, "\n\n")))
	_, _ = m.Reply(ctx, reply)
}

func truncateOwnerEvalReply(reply string) string {
	if len(reply) <= ownerEvalMaxReplySize {
		return reply
	}
	suffix := "\n\n..."
	if i := strings.LastIndex(reply, "[output dipotong]"); i >= 0 {
		suffix = "\n\n... [output dipotong]"
		reply = reply[:i]
	}
	limit := ownerEvalMaxReplySize - len(suffix)
	for len(reply) > limit {
		_, size := utf8.DecodeLastRuneInString(reply)
		reply = reply[:len(reply)-size]
	}
	return reply + suffix
}

func formatEvalValue(val any) string {
	if val == nil {
		return "nil"
	}
	switch v := val.(type) {
	case string:
		if pretty, ok := tryPrettyJSON(v); ok {
			return pretty
		}
		return v
	case []byte:
		if pretty, ok := tryPrettyJSON(string(v)); ok {
			return pretty
		}
		return string(v)
	case error:
		return v.Error()
	}
	return Inspect(val, 2)
}

func Inspect(val any, maxDepth int) string {
	visited := make(map[uintptr]bool)
	out := inspectValue(reflect.ValueOf(val), 0, maxDepth, "  ", visited)
	switch reflect.TypeOf(val).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		return "```\n" + out + "\n```"
	default:
		return out
	}
}

func inspectValue(v reflect.Value, depth, maxDepth int, indent string, visited map[uintptr]bool) string {
	if !v.IsValid() {
		return "nil"
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "nil"
		}
		if v.Kind() == reflect.Pointer {
			ptr := v.Pointer()
			if visited[ptr] {
				return "[Circular]"
			}
			visited[ptr] = true
			defer delete(visited, ptr)
			return "&" + inspectValue(v.Elem(), depth, maxDepth, indent, visited)
		}
		return inspectValue(v.Elem(), depth, maxDepth, indent, visited)
	}

	curIndent := strings.Repeat(indent, depth)
	nextIndent := strings.Repeat(indent, depth+1)
	switch v.Kind() {
	case reflect.String:
		return strconv.Quote(v.String())
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Func:
		return "[Function]"
	case reflect.Chan:
		return "[Channel]"
	case reflect.Slice, reflect.Array:

		if v.Len() == 0 {
			return "[]"
		}
		if depth >= maxDepth {
			return fmt.Sprintf("[Array(%d)]", v.Len())
		}
		items := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			items = append(items, nextIndent+inspectValue(v.Index(i), depth+1, maxDepth, indent, visited))
		}
		return "[\n" + strings.Join(items, ",\n") + "\n" + curIndent + "]"
	case reflect.Map:
		if v.Len() == 0 {
			return "{}"
		}
		if depth >= maxDepth {
			return fmt.Sprintf("[Map(%d)]", v.Len())
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			kStr := fmt.Sprint(k.Interface())
			if k.Kind() != reflect.String {
				kStr = inspectValue(k, depth+1, maxDepth, indent, visited)
			}
			valStr := inspectValue(v.MapIndex(k), depth+1, maxDepth, indent, visited)
			pairs = append(pairs, nextIndent+kStr+": "+valStr)
		}
		return "{\n" + strings.Join(pairs, ",\n") + "\n" + curIndent + "}"
	case reflect.Struct:
		t := v.Type()
		if depth >= maxDepth {
			name := t.Name()
			if name == "" {
				name = "Struct"
			}
			return fmt.Sprintf("[%s]", name)
		}
		fields := make([]string, 0, v.NumField())
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			fv := v.Field(i)
			if fv.CanInterface() {
				fields = append(fields, nextIndent+f.Name+": "+inspectValue(fv, depth+1, maxDepth, indent, visited))
			} else {
				fields = append(fields, nextIndent+f.Name+": [Unexported]")
			}
		}
		prefix := ""
		if t.Name() != "" {
			prefix = t.Name() + " "
		}
		if len(fields) == 0 {
			return prefix + "{}"
		}
		return prefix + "{\n" + strings.Join(fields, ",\n") + "\n" + curIndent + "}"
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

func tryPrettyJSON(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 2 || len(trimmed) > ownerEvalMaxOutputSize {
		return "", false
	}
	first, last := trimmed[0], trimmed[len(trimmed)-1]
	if (first != '{' || last != '}') && (first != '[' || last != ']') {
		return "", false
	}
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(trimmed), "", "  "); err != nil {
		return "", false
	}
	return "```json\n" + out.String() + "\n```", true
}

func init() {
	commands.Register(&commands.Command{
		Name:        ">",
		Description: "Evaluasi kode Go owner menggunakan Yaegi (akses penuh)",
		As:          []string{"=>"},
		Tags:        "owner",
		IsOwner:     true,
		IsQuery:     true,
		Exec:        ownerEval,
	})
}
