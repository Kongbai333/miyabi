package frame_test

import (
	"bytes"
	"context"
	stdimage "image"
	_ "image/jpeg"
	"os/exec"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/frame"
)

// The decoder is an external process, so these tests need ffmpeg. They skip
// rather than fail where it is absent, which keeps `go test ./...` usable
// without the runtime image.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(frame.Binary); err != nil {
		t.Skip("ffmpeg is not installed")
	}
}

// segment produces one self-contained video segment, the same shape the frame
// task decodes: a stream that starts at a keyframe and can be read from a pipe.
func segment(t *testing.T) []byte {
	t.Helper()
	command := exec.Command(frame.Binary,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10",
		"-c:v", "mpeg4", "-f", "mpegts", "pipe:1",
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("build fixture segment: %v: %s", err, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("fixture segment is empty")
	}
	return stdout.Bytes()
}

func TestExtractDecodesAJPEGFrame(t *testing.T) {
	requireFFmpeg(t)
	still, err := frame.Extract(t.Context(), bytes.NewReader(segment(t)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, format, err := stdimage.Decode(bytes.NewReader(still))
	if err != nil || format != "jpeg" {
		t.Fatalf("frame is not a decodable JPEG: format=%q err=%v", format, err)
	}
	if size := decoded.Bounds().Size(); size.X != 320 || size.Y != 240 {
		t.Fatalf("frame kept the source size: %v", size)
	}
}

func TestExtractReportsWhyTheStreamCouldNotBeDecoded(t *testing.T) {
	requireFFmpeg(t)
	_, err := frame.Extract(t.Context(), strings.NewReader("this is not a video stream"))
	if err == nil {
		t.Fatal("a non-video stream was accepted")
	}
	// ffmpeg's own reason has to survive, or a failed task says nothing useful.
	if !strings.Contains(err.Error(), "decode video frame") || !strings.Contains(err.Error(), "Invalid data") {
		t.Fatalf("error lost the decoder's reason: %v", err)
	}
}

func TestExtractStopsWhenTheContextIsCancelled(t *testing.T) {
	requireFFmpeg(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := frame.Extract(ctx, bytes.NewReader(segment(t))); err == nil {
		t.Fatal("a cancelled extraction reported success")
	}
}
