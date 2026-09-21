package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/frame"
	"github.com/ppxb/miyabi/internal/pan"
)

// framePayload identifies the movie whose card still needs an image.
type framePayload struct {
	Source     LibrarySource `json:"source"`
	ScanTaskID int           `json:"scan_task_id,omitempty"`
	MovieID    int           `json:"movie_id"`
}

const (
	// maxFramePlaylist bounds a playlist document.
	maxFramePlaylist = 2 << 20
	// maxFrameSegment bounds one transcoded segment. A segment is a few hundred
	// kilobytes; the limit only stops a mislabelled response.
	maxFrameSegment = 16 << 20
)

// Frame gives a movie whose metadata scrape failed a card image taken from its
// own video. The source file is read as one transcoded HLS segment rather than
// as a byte range of the original, because a prefix of an mp4 whose index sits
// at the end cannot be decoded at all.
func (service *ScrapeService) Frame(ctx context.Context, job TaskJob) error {
	input, err := decodeTaskPayload[framePayload](job.Payload)
	if err != nil {
		return err
	}
	segment, err := service.stillSegment(ctx, input)
	if err != nil {
		return err
	}
	still, err := frame.Extract(ctx, bytes.NewReader(segment))
	if err != nil {
		return err
	}
	artwork, err := service.images.FromFrame(still)
	if err != nil {
		return err
	}
	// Only ever fill an empty card: a scrape that succeeded meanwhile, or one
	// still on its way, owns the artwork instead.
	updated, err := service.library.database.Movie.Update().Where(
		movie.IDEQ(input.MovieID), movie.HasFilesWith(libraryFiles(input.Source)),
		movie.Or(movie.CoverIsNil(), movie.CoverEQ("")),
	).SetCover(artwork.Thumbnail).SetFanarts([]string{artwork.Fanart}).Save(ctx)
	if err != nil {
		return fmt.Errorf("save video still: %w", err)
	}
	if updated > 0 {
		service.library.tasks.NotifyLibraryChanged()
	}
	return nil
}

// stillSegment downloads one segment of the movie's transcoded stream.
func (service *ScrapeService) stillSegment(ctx context.Context, input framePayload) ([]byte, error) {
	state, err := service.library.drive.verifiedSource(ctx)
	if err != nil {
		return nil, err
	}
	source := state.source()
	if source.AccountID != input.Source.AccountID || source.Directory.ID != input.Source.Directory.ID {
		return nil, ErrSourceChanged
	}
	// A movie split into parts uses its largest file, which is the feature.
	entry, err := service.library.database.File.Query().
		Where(libraryFiles(input.Source), file.MovieIDEQ(input.MovieID)).
		Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldID)).
		Select(file.FieldID, file.FieldName, file.FieldPickCode).First(ctx)
	if err != nil {
		return nil, fmt.Errorf("find video for still: %w", err)
	}
	if entry.PickCode == "" {
		return nil, domain.E(domain.KindInvalid, "媒体文件缺少 115 提取码，请重新扫描后再试", nil)
	}
	address, err := withPanSourceToken(ctx, service.library.drive, state, func(token string) (string, error) {
		sources, err := service.library.drive.client.PlayURL(ctx, token, entry.PickCode)
		if err != nil {
			return "", err
		}
		return highestSource(sources), nil
	})
	if err != nil {
		return nil, err
	}
	if err := service.library.checkScanSource(input.Source, state.authorizationVersion); err != nil {
		return nil, err
	}
	playlist, base, err := service.mediaPlaylist(ctx, address)
	if err != nil {
		return nil, err
	}
	segment, found := playlist.stillSegment()
	if !found {
		return nil, domain.E(domain.KindUpstream, "115 暂未提供该文件的转码分片，请稍后重试", nil)
	}
	// Segment URIs are usually relative to the playlist that named them.
	reference, err := url.Parse(segment.uri)
	if err != nil {
		return nil, domain.E(domain.KindUpstream, "115 返回的分片地址无效", err)
	}
	headers := http.Header{}
	if segment.length > 0 {
		headers.Set("Range", fmt.Sprintf("bytes=%d-%d", segment.start, segment.start+segment.length-1))
	}
	body, err := service.readMedia(ctx, base.ResolveReference(reference).String(), headers, maxFrameSegment)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func highestSource(sources []pan.PlaySource) string {
	best := sources[0]
	for _, source := range sources {
		if source.Height > best.Height {
			best = source
		}
	}
	return best.URL
}

// mediaPlaylist resolves a playback URL down to a media playlist. 115 hands out
// either a master playlist or the media playlist directly, so at most one
// rendition level is followed.
func (service *ScrapeService) mediaPlaylist(ctx context.Context, address string) (hlsPlaylist, *url.URL, error) {
	for range 2 {
		body, final, err := service.readMediaWithBase(ctx, address, nil, maxFramePlaylist)
		if err != nil {
			return hlsPlaylist{}, nil, err
		}
		playlist, err := parseHLSPlaylist(body)
		if err != nil {
			return hlsPlaylist{}, nil, err
		}
		if len(playlist.segments) > 0 {
			return playlist, final, nil
		}
		variant := playlist.bestVariant()
		if variant == "" {
			return hlsPlaylist{}, nil, domain.E(domain.KindUpstream, "115 返回的播放列表既没有分片也没有清晰度选项", nil)
		}
		reference, err := url.Parse(variant)
		if err != nil {
			return hlsPlaylist{}, nil, domain.E(domain.KindUpstream, "115 返回的播放列表包含无效地址", err)
		}
		address = final.ResolveReference(reference).String()
	}
	return hlsPlaylist{}, nil, domain.E(domain.KindUpstream, "115 返回的播放列表嵌套层级异常", nil)
}

func (service *ScrapeService) readMedia(ctx context.Context, address string, headers http.Header, limit int64) ([]byte, error) {
	body, _, err := service.readMediaWithBase(ctx, address, headers, limit)
	return body, err
}

// readMediaWithBase reads a bounded response and reports the URL it settled on,
// which is what a relative playlist URI resolves against.
func (service *ScrapeService) readMediaWithBase(ctx context.Context, address string, headers http.Header, limit int64) ([]byte, *url.URL, error) {
	response, err := service.library.drive.client.OpenMedia(ctx, http.MethodGet, address, headers)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
	default:
		if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized {
			return nil, nil, domain.E(domain.KindConflict, "播放地址已失效，请稍后重试", nil)
		}
		return nil, nil, domain.E(domain.KindUpstream, "115 视频流异常，请稍后重试",
			fmt.Errorf("upstream returned HTTP %d", response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read 115 media: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, nil, fmt.Errorf("115 media exceeds %d bytes", limit)
	}
	return body, response.Request.URL, nil
}

// enqueueFrameTask gives a movie whose metadata scrape failed a card image from
// its own video. A movie that already has artwork, or a still already on its
// way, is left alone.
func enqueueFrameTask(ctx context.Context, tx *ent.Tx, input metadataPayload) error {
	pending, err := tx.Task.Query().Where(
		task.TypeEQ("frame"), task.StatusIn(task.StatusQueued, task.StatusRunning),
		func(selector *sql.Selector) {
			selector.Where(sqljson.ValueEQ(task.FieldPayload, input.MovieID, sqljson.Path("movie_id")))
		},
	).Exist(ctx)
	if err != nil {
		return fmt.Errorf("find queued video still: %w", err)
	}
	if pending {
		return nil
	}
	// A task payload can outlive the movie it references, and the surrounding
	// update treats a missing movie as a no-op, so this has to as well.
	records, err := tx.Movie.Query().Where(movie.IDEQ(input.MovieID)).Select(movie.FieldCover).All(ctx)
	if err != nil {
		return err
	}
	if len(records) == 0 || valueOrZero(records[0].Cover) != "" {
		return nil
	}
	payload, err := encodeTaskPayload(framePayload{Source: input.Source, ScanTaskID: input.ScanTaskID, MovieID: input.MovieID})
	if err != nil {
		return err
	}
	if err := tx.Task.Create().SetType("frame").SetPayload(payload).Exec(ctx); err != nil {
		return fmt.Errorf("queue video still: %w", err)
	}
	return nil
}
