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

	"github.com/MapIHS/kotonehara/internal/clients"
	"github.com/MapIHS/kotonehara/internal/commands"
	"github.com/MapIHS/kotonehara/internal/infra/config"
	"github.com/MapIHS/kotonehara/internal/message"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const (
	ownerEvalTimeout      = 30 * time.Second
	ownerEvalMaxReplySize = 65536
)

func ownerEval(ctx context.Context, client *clients.Client, m *message.Message, cfg config.Config) {
	query := strings.TrimSpace(m.Query)
	if query == "" {
		_, _ = m.Reply(ctx, "Kode Go dibutuhkan, yaa.\n\nContoh:\n> 1 + 1\n> time.Now().Format(time.RFC3339)\n> strings.ToUpper(\"halo\")\n> Msg.Sender.User")
		return
	}

	if pretty, ok := tryPrettyJSON(query); ok {
		_, _ = m.Reply(ctx, pretty)
		return
	}

	var stdout, stderr bytes.Buffer
	i := interp.New(interp.Options{
		Stdout:       &stdout,
		Stderr:       &stderr,
		Unrestricted: true,
	})

	// Load standard library symbols into the interpreter
	_ = i.Use(stdlib.Symbols)

	// Load custom symbols for the bot context into the interpreter
	_ = i.Use(interp.Exports{
		"bot/bot": {
			"Client": reflect.ValueOf(client),
			"M":      reflect.ValueOf(m),
			"Ctx":    reflect.ValueOf(ctx),
			"Cfg":    reflect.ValueOf(cfg),
		},
	})

	// Pre-import commonly used packages for convenience
	_, _ = i.Eval(`import (
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
	)`)

	evalCtx, cancel := context.WithTimeout(ctx, ownerEvalTimeout)
	defer cancel()

	res, err := i.EvalWithContext(evalCtx, query)

	if errors.Is(evalCtx.Err(), context.DeadlineExceeded) {
		_, _ = m.Reply(ctx, "Evaluasi Go timeout (30 detik), yaa.")
		return
	}

	var parts []string

	outText := strings.TrimSpace(stdout.String())
	errText := strings.TrimSpace(stderr.String())

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
		val := res.Interface()
		valStr := formatEvalValue(val)

		if outText == "" || (valStr != "" && valStr != "<nil>") {
			parts = append(parts, valStr)
		}
	} else if len(parts) == 0 {
		parts = append(parts, "nil")
	}

	reply := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if len(reply) > ownerEvalMaxReplySize {
		reply = reply[:ownerEvalMaxReplySize-10] + "\n\n..."
	}

	_, _ = m.Reply(ctx, reply)
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

	res := Inspect(val, 2)
	return res
}

func Inspect(val any, maxDepth int) string {
	visited := make(map[uintptr]bool)
	return inspectValue(reflect.ValueOf(val), 0, maxDepth, "  ", visited)
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
		var items []string
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
		var pairs []string
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
		var fields []string
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
	if len(trimmed) < 2 {
		return "", false
	}
	first, last := trimmed[0], trimmed[len(trimmed)-1]
	if (first != '{' || last != '}') && (first != '[' || last != ']') {
		return "", false
	}

	var out bytes.Buffer
	if err := json.Indent(&out, []byte(trimmed), "", "  "); err == nil {
		return out.String(), true
	}
	return "", false
}

func init() {
	commands.Register(&commands.Command{
		Name:        ">",
		Description: "Evaluasi kode Go runtime menggunakan Yaegi (full akses)",
		As:          []string{">", "=>"},
		Tags:        "owner",
		IsPrefix:    false,
		IsOwner:     true,
		IsQuery:     true,
		Exec:        ownerEval,
	})
}
