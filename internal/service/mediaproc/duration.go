package mediaproc

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func ProbeDuration(ctx context.Context, path string) (time.Duration, error) {
	out, err := CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries",
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || math.IsNaN(seconds) || seconds <= 0 || seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return 0, fmt.Errorf("durasi media invalid: %q", strings.TrimSpace(string(out)))
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
