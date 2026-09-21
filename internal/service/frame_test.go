package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/frame"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
)

const (
	fixtureMasterPlaylist = "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360\nlow.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=4000000,RESOLUTION=1920x1080\nbest.m3u8\n"
	fixtureMediaPlaylist = "#EXTM3U\n#EXT-X-TARGETDURATION:4\n" +
		"#EXTINF:4.000000,\nseg000.ts\n" +
		"#EXTINF:4.000000,\nseg001.ts\n" +
		"#EXTINF:4.000000,\nseg002.ts\n" +
		"#EXTINF:4.000000,\nseg003.ts\n" +
		"#EXT-X-ENDLIST\n"
)

func TestParseHLSPlaylistReadsVariantsAndSegments(t *testing.T) {
	master, err := parseHLSPlaylist([]byte(fixtureMasterPlaylist))
	if err != nil {
		t.Fatal(err)
	}
	if len(master.segments) != 0 || len(master.variants) != 2 {
		t.Fatalf("master playlist = %+v", master)
	}
	// The still is taken from the sharpest rendition the playlist offers.
	if best := master.bestVariant(); best != "best.m3u8" {
		t.Fatalf("best variant = %q", best)
	}

	media, err := parseHLSPlaylist([]byte(fixtureMediaPlaylist))
	if err != nil {
		t.Fatal(err)
	}
	if len(media.segments) != 4 || media.segments[0].uri != "seg000.ts" || media.segments[3].uri != "seg003.ts" {
		t.Fatalf("media playlist = %+v", media)
	}
	// A quarter in avoids the opening title card without downloading the file.
	still, found := media.stillSegment()
	if !found || still.uri != "seg001.ts" {
		t.Fatalf("still segment = %+v", still)
	}
	if len(media.variants) != 0 {
		t.Fatalf("media playlist invented variants: %+v", media.variants)
	}
}

func TestParseHLSPlaylistResolvesByteRanges(t *testing.T) {
	playlist, err := parseHLSPlaylist([]byte("#EXTM3U\n" +
		"#EXTINF:4.000000,\n#EXT-X-BYTERANGE:1000@0\nseg.ts\n" +
		"#EXTINF:4.000000,\n#EXT-X-BYTERANGE:1000\nseg.ts\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(playlist.segments) != 2 {
		t.Fatalf("segments = %+v", playlist.segments)
	}
	if first := playlist.segments[0]; first.start != 0 || first.length != 1000 {
		t.Fatalf("first range = %+v", first)
	}
	// A range without an offset continues where the previous one ended.
	if second := playlist.segments[1]; second.start != 1000 || second.length != 1000 {
		t.Fatalf("second range = %+v", second)
	}
}

func TestParseHLSPlaylistRejectsBrokenByteRanges(t *testing.T) {
	for _, body := range []string{
		"#EXTM3U\n#EXTINF:4.0,\n#EXT-X-BYTERANGE:1000\nseg.ts\n",  // no offset to continue from
		"#EXTM3U\n#EXTINF:4.0,\n#EXT-X-BYTERANGE:0@0\nseg.ts\n",   // empty range
		"#EXTM3U\n#EXTINF:4.0,\n#EXT-X-BYTERANGE:10@x\nseg.ts\n",  // unparsable offset
		"#EXTM3U\n#EXTINF:4.0,\n#EXT-X-BYTERANGE:abc@0\nseg.ts\n", // unparsable length
		"#EXTM3U\n#EXTINF:4.0,\n#EXT-X-BYTERANGE:10@-4\nseg.ts\n", // negative offset
		"#EXTM3U\n#EXTINF:4.0,\n#EXT-X-BYTERANGE:-10@0\nseg.ts\n", // negative length
	} {
		if _, err := parseHLSPlaylist([]byte(body)); err == nil {
			t.Fatalf("byte range was accepted: %q", body)
		}
	}
}

func TestFailedScrapeQueuesAVideoStill(t *testing.T) {
	library, parent, payload := libraryFixture(t)
	ctx := t.Context()
	film := library.database.Movie.Create().SetCode("ABP-001").SaveX(ctx)
	library.database.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film).ExecX(ctx)

	finishScrape := func() {
		t.Helper()
		body, err := encodeTaskPayload(metadataPayload{
			Source: payload.Source, ScanTaskID: parent.ID, MovieID: film.ID, Code: film.Code,
		})
		if err != nil {
			t.Fatal(err)
		}
		record, err := library.database.Task.Create().SetType("scrape").SetPayload(body).Save(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := library.tasks.Finish(ctx, record.ID, errors.New("fixture scrape failure")); err != nil {
			t.Fatal(err)
		}
	}

	finishScrape()
	stills := library.database.Task.Query().Where(task.TypeEQ("frame")).AllX(ctx)
	if len(stills) != 1 {
		t.Fatalf("failed scrape queued %d stills, want 1", len(stills))
	}
	still, err := decodeTaskPayload[framePayload](stills[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if still.MovieID != film.ID || still.Source != payload.Source || still.ScanTaskID != parent.ID {
		t.Fatalf("still payload = %+v", still)
	}
	if library.database.Movie.GetX(ctx, film.ID).ScrapeStatus != movie.ScrapeStatusFailed {
		t.Fatal("the movie was not marked as failed")
	}

	// Another failed scrape for the same movie must not pile up duplicates.
	finishScrape()
	if count := library.database.Task.Query().Where(task.TypeEQ("frame")).CountX(ctx); count != 1 {
		t.Fatalf("a second failure queued %d stills", count)
	}
}

func TestFailedScrapeSkipsAMovieThatAlreadyHasArtwork(t *testing.T) {
	library, parent, payload := libraryFixture(t)
	ctx := t.Context()
	film := library.database.Movie.Create().SetCode("ABP-001").SetCover("/api/library/artwork/aa.jpg").SaveX(ctx)
	library.database.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film).ExecX(ctx)
	body, err := encodeTaskPayload(metadataPayload{Source: payload.Source, ScanTaskID: parent.ID, MovieID: film.ID})
	if err != nil {
		t.Fatal(err)
	}
	record, err := library.database.Task.Create().SetType("cover").SetPayload(body).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := library.tasks.Finish(ctx, record.ID, errors.New("fixture cover failure")); err != nil {
		t.Fatal(err)
	}
	if count := library.database.Task.Query().Where(task.TypeEQ("frame")).CountX(ctx); count != 0 {
		t.Fatalf("a movie with artwork queued %d stills", count)
	}
}

// A task can outlive the movie it references; the surrounding update treats that
// as a no-op, so queueing must not turn it into a failure.
func TestFailedScrapeToleratesAMissingMovie(t *testing.T) {
	library, parent, payload := libraryFixture(t)
	ctx := t.Context()
	body, err := encodeTaskPayload(metadataPayload{Source: payload.Source, ScanTaskID: parent.ID, MovieID: 4242})
	if err != nil {
		t.Fatal(err)
	}
	record, err := library.database.Task.Create().SetType("scrape").SetPayload(body).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := library.tasks.Finish(ctx, record.ID, errors.New("fixture scrape failure")); err != nil {
		t.Fatal(err)
	}
	if count := library.database.Task.Query().Where(task.TypeEQ("frame")).CountX(ctx); count != 0 {
		t.Fatalf("a removed movie queued %d stills", count)
	}
}

func TestFrameFillsAnEmptyCardFromTheTranscodedStream(t *testing.T) {
	requireFFmpeg(t)
	library, client := panConcurrencyFixture(t)
	ctx := t.Context()
	source := library.drive.snapshot().source()
	film := library.database.Movie.Create().SetCode("ABP-001").SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)
	library.database.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetPickCode("pc-video").SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).ExecX(ctx)

	requested := serveStillStream(t, client)
	if err := runFrameTask(t, library, source, film.ID); err != nil {
		t.Fatal(err)
	}

	updated := library.database.Movie.GetX(ctx, film.ID)
	if updated.Cover == nil || !strings.HasPrefix(*updated.Cover, mediaimage.URLPrefix) {
		t.Fatalf("the card kept no still: %+v", updated.Cover)
	}
	if len(updated.Fanarts) != 1 {
		t.Fatalf("the still is not available to the hover card: %+v", updated.Fanarts)
	}
	// A still is not a poster, and the badge must keep saying the scrape failed.
	if updated.Poster != nil {
		t.Fatalf("a video still was stored as a poster: %v", *updated.Poster)
	}
	if updated.ScrapeStatus != movie.ScrapeStatusFailed {
		t.Fatalf("the scrape status changed: %v", updated.ScrapeStatus)
	}
	if body, err := library.images.Read(strings.TrimPrefix(*updated.Cover, mediaimage.URLPrefix)); err != nil || len(body) == 0 {
		t.Fatalf("the still was not cached: %v", err)
	}
	// The master playlist is followed to the sharpest rendition, and only the
	// chosen segment is downloaded.
	want := []string{"master.m3u8", "best.m3u8", "seg001.ts"}
	if len(*requested) != len(want) {
		t.Fatalf("media requests = %v", *requested)
	}
	for index, fragment := range want {
		if !strings.Contains((*requested)[index], fragment) {
			t.Fatalf("media requests = %v", *requested)
		}
	}
}

func TestFrameKeepsAnArtworkThatArrivedFirst(t *testing.T) {
	requireFFmpeg(t)
	library, client := panConcurrencyFixture(t)
	ctx := t.Context()
	source := library.drive.snapshot().source()
	existing := mediaimage.URLPrefix + strings.Repeat("a", 64)
	film := library.database.Movie.Create().SetCode("ABP-001").SetCover(existing).SaveX(ctx)
	library.database.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetPickCode("pc-video").SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).ExecX(ctx)

	serveStillStream(t, client)
	if err := runFrameTask(t, library, source, film.ID); err != nil {
		t.Fatal(err)
	}
	if cover := library.database.Movie.GetX(ctx, film.ID).Cover; cover == nil || *cover != existing {
		t.Fatalf("an existing cover was replaced: %v", cover)
	}
}

func TestFrameReportsAStreamWithoutSegments(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	source := library.drive.snapshot().source()
	film := library.database.Movie.Create().SetCode("ABP-001").SaveX(t.Context())
	library.database.File.Create().SetFileID("video").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetPickCode("pc-video").SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(film).ExecX(t.Context())
	client.playURL = func(context.Context, string, string) ([]pan.PlaySource, error) {
		return []pan.PlaySource{{URL: "https://cdn.example/master.m3u8", Height: 1080}}, nil
	}
	client.openMedia = func(_ context.Context, _, address string, _ http.Header) (*http.Response, error) {
		return mediaResponse(address, "application/vnd.apple.mpegurl", "#EXTM3U\n#EXT-X-ENDLIST\n"), nil
	}
	err := runFrameTask(t, library, source, film.ID)
	if err == nil || !strings.Contains(err.Error(), "播放列表") {
		t.Fatalf("an empty playlist was accepted: %v", err)
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(frame.Binary); err != nil {
		t.Skip("ffmpeg is not installed")
	}
}

func runFrameTask(t *testing.T, library *LibraryService, source LibrarySource, movieID int) error {
	t.Helper()
	payload, err := encodeTaskPayload(framePayload{Source: source, MovieID: movieID})
	if err != nil {
		t.Fatal(err)
	}
	scrape := NewScrapeService(library, nil, library.images)
	return scrape.Frame(t.Context(), TaskJob{ID: 1, Type: "frame", Payload: payload})
}

// serveStillStream points the stub client at a master playlist, its best
// rendition and one segment, and returns the addresses it was asked for.
func serveStillStream(t *testing.T, client *panStub) *[]string {
	t.Helper()
	segment := fixtureSegment(t)
	client.playURL = func(context.Context, string, string) ([]pan.PlaySource, error) {
		return []pan.PlaySource{
			{URL: "https://cdn.example/low.m3u8", Height: 360},
			{URL: "https://cdn.example/master.m3u8", Height: 1080},
		}, nil
	}
	requested := &[]string{}
	client.openMedia = func(_ context.Context, method, address string, _ http.Header) (*http.Response, error) {
		*requested = append(*requested, method+" "+address)
		switch address {
		case "https://cdn.example/master.m3u8":
			return mediaResponse(address, "application/vnd.apple.mpegurl", fixtureMasterPlaylist), nil
		case "https://cdn.example/best.m3u8":
			return mediaResponse(address, "application/vnd.apple.mpegurl", fixtureMediaPlaylist), nil
		case "https://cdn.example/seg001.ts":
			return mediaResponse(address, "video/mp2t", string(segment)), nil
		}
		return nil, fmt.Errorf("unexpected address %q", address)
	}
	return requested
}

// fixtureSegment builds one self-contained video segment with the decoder the
// task already requires, so no binary fixture is carried in the repository.
func fixtureSegment(t *testing.T) []byte {
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
	return stdout.Bytes()
}

func mediaResponse(address, contentType, body string) *http.Response {
	request, _ := http.NewRequest(http.MethodGet, address, nil)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
