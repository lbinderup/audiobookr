package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FileInfo is what the pipeline needs to know about one input file.
type FileInfo struct {
	Path        string
	DurationMs  int64
	BitrateKbps int
	SampleRate  int
	Channels    int
	Codec       string // e.g. "mp3", "aac", "flac"
	Container   string // e.g. "mov,mp4,m4a,3gp,3g2,mj2", "mp3"
	Chapters    []ProbedChapter
}

type ProbedChapter struct {
	Title   string
	StartMs int64
	EndMs   int64
}

// IsAACInMP4 reports whether the file can be stream-copied into an m4b.
func (f FileInfo) IsAACInMP4() bool {
	return f.Codec == "aac" && strings.Contains(f.Container, "mp4")
}

// ProbeFile inspects one audio file with ffprobe — exported for the web
// layer's chapter-preview feature.
func ProbeFile(ctx context.Context, ffprobePath, path string) (*FileInfo, error) {
	return prober{ffprobe: ffprobePath}.probe(ctx, path)
}

type prober struct {
	ffprobe string
}

func (p prober) probe(ctx context.Context, path string) (*FileInfo, error) {
	out, err := exec.CommandContext(ctx, p.ffprobe,
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters",
		path,
	).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ffprobe %s: %s", path, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var raw struct {
		Format struct {
			FormatName string            `json:"format_name"`
			Duration   string            `json:"duration"`
			BitRate    string            `json:"bit_rate"`
			Tags       map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			BitRate    string `json:"bit_rate"`
		} `json:"streams"`
		Chapters []struct {
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			Tags      struct {
				Title string `json:"title"`
			} `json:"tags"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ffprobe %s: parse: %w", path, err)
	}

	info := &FileInfo{
		Path:       path,
		Container:  raw.Format.FormatName,
		DurationMs: secondsToMs(raw.Format.Duration),
	}
	if kb := parseBitrateKbps(raw.Format.BitRate); kb > 0 {
		info.BitrateKbps = kb
	}
	for _, s := range raw.Streams {
		if s.CodecType != "audio" {
			continue
		}
		info.Codec = s.CodecName
		info.Channels = s.Channels
		info.SampleRate, _ = strconv.Atoi(s.SampleRate)
		if kb := parseBitrateKbps(s.BitRate); kb > 0 {
			info.BitrateKbps = kb // stream bitrate beats container bitrate
		}
		break
	}
	if info.Codec == "" {
		return nil, fmt.Errorf("%s has no audio stream", path)
	}
	for _, c := range raw.Chapters {
		info.Chapters = append(info.Chapters, ProbedChapter{
			Title:   c.Tags.Title,
			StartMs: secondsToMs(c.StartTime),
			EndMs:   secondsToMs(c.EndTime),
		})
	}
	return info, nil
}

func secondsToMs(s string) int64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1000)
}

func parseBitrateKbps(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n / 1000
}

// standardBitrates are the rungs we snap a probed bitrate to.
var standardBitrates = []int{64, 96, 128, 192, 256, 320}

// SnapBitrate picks the nearest standard rung; 0 or negative falls back to 128.
func SnapBitrate(kbps int) int {
	if kbps <= 0 {
		return 128
	}
	best, bestDiff := standardBitrates[0], 1<<30
	for _, r := range standardBitrates {
		d := r - kbps
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			best, bestDiff = r, d
		}
	}
	return best
}
