// Package frame turns a self-contained video stream into one still image.
//
// Decoding H.264 in Go is not practical, so the decoder is ffmpeg. It is the
// only external process this project runs.
package frame

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Binary is the executable that decodes the stream.
const Binary = "ffmpeg"

const (
	// maxFrameSize stops a malformed stream from filling memory. A 4K still is
	// well under this.
	maxFrameSize = 8 << 20
	// maxDiagnostic keeps the reason ffmpeg refused, without letting a noisy
	// stream bloat a stored task error.
	maxDiagnostic = 2 << 10
)

// Extract decodes the first frame of video as a JPEG. The stream must start at
// a keyframe - one HLS segment, for example - because ffmpeg cannot seek inside
// a pipe.
func Extract(ctx context.Context, video io.Reader) ([]byte, error) {
	command := exec.CommandContext(ctx, Binary,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "image2", "-vcodec", "mjpeg",
		"pipe:1",
	)
	command.Stdin = video
	output := &cappedBuffer{limit: maxFrameSize}
	diagnostic := &bytes.Buffer{}
	command.Stdout = output
	command.Stderr = truncatingWriter{buffer: diagnostic, limit: maxDiagnostic}
	if err := command.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("ffmpeg is not installed: %w", err)
		}
		if reason := strings.TrimSpace(diagnostic.String()); reason != "" {
			return nil, fmt.Errorf("decode video frame: %w: %s", err, reason)
		}
		return nil, fmt.Errorf("decode video frame: %w", err)
	}
	still := output.Bytes()
	if !isJPEG(still) {
		return nil, fmt.Errorf("ffmpeg returned %d bytes that are not a JPEG", len(still))
	}
	return still, nil
}

func isJPEG(body []byte) bool {
	return len(body) > 3 && body[0] == 0xFF && body[1] == 0xD8 && body[2] == 0xFF
}

// cappedBuffer fails the command once a still exceeds the limit, instead of
// buffering an unbounded amount of output.
type cappedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (capped *cappedBuffer) Write(part []byte) (int, error) {
	if capped.buffer.Len()+len(part) > capped.limit {
		return 0, fmt.Errorf("video frame exceeds %d bytes", capped.limit)
	}
	return capped.buffer.Write(part)
}

func (capped *cappedBuffer) Bytes() []byte { return capped.buffer.Bytes() }

// truncatingWriter keeps the first bytes of stderr and reports every write as
// accepted, so a chatty decoder cannot fail the command it is explaining.
type truncatingWriter struct {
	buffer *bytes.Buffer
	limit  int
}

func (writer truncatingWriter) Write(part []byte) (int, error) {
	if remaining := writer.limit - writer.buffer.Len(); remaining > 0 {
		kept := part
		if len(kept) > remaining {
			kept = kept[:remaining]
		}
		writer.buffer.Write(kept)
	}
	return len(part), nil
}
