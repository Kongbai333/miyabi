package service

import (
	"context"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

// A directory's only sidecar has the last word on its catalogue number. The
// release groups prefix the publisher's digits (200GANA-3458) or name a date
// code with their studio (CARIB-060326-001), and JavDB lists neither spelling, so
// the number the library searches for has to come from the sidecar instead.
func TestResolveSingleNFOTakesTheNumberTheSidecarCarries(t *testing.T) {
	for _, test := range []struct {
		name     string
		sidecars []pan.File
		document string
		videos   []scanVideo
		want     []string
		reads    int
	}{
		{
			name:     "distributor prefix",
			sidecars: []pan.File{{ID: "nfo", Name: "GANA-3458.nfo", PickCode: "nfo"}},
			document: "GANA-3458",
			videos: []scanVideo{
				{File: pan.File{ID: "v1", Name: "4k688.com@200GANA-3458.mp4", Size: 1 << 30}, Code: "200GANA-3458"},
				{File: pan.File{ID: "v2", Name: "APP.mp4", Size: 5 << 20}},
			},
			want: []string{"GANA-3458", ""},
		},
		{
			name:     "studio date prefix",
			sidecars: []pan.File{{ID: "nfo", Name: "060326-001.nfo", PickCode: "nfo"}},
			document: "060326-001",
			videos: []scanVideo{
				{File: pan.File{ID: "v1", Name: "Carib-060326-001.mp4", Size: 1 << 30}, Code: "CARIB-060326-001"},
			},
			want: []string{"060326-001"},
		},
		{
			name:     "feature without a number in its name",
			sidecars: []pan.File{{ID: "nfo", Name: "movie.nfo", PickCode: "nfo"}},
			document: "ABP-001",
			videos: []scanVideo{
				{File: pan.File{ID: "v1", Name: "feature.mp4", Size: 1 << 30}},
				{File: pan.File{ID: "v2", Name: "trailer.mp4", Size: 10 << 20}},
			},
			want:  []string{"ABP-001", ""},
			reads: 1,
		},
		{
			name:     "two films keep their own numbers",
			sidecars: []pan.File{{ID: "nfo", Name: "movie.nfo", PickCode: "nfo"}},
			document: "ABP-001",
			videos: []scanVideo{
				{File: pan.File{ID: "v1", Name: "ABP-001.mp4", Size: 1 << 30}, Code: "ABP-001"},
				{File: pan.File{ID: "v2", Name: "IPX-123.mp4", Size: 1 << 30}, Code: "IPX-123"},
			},
			want: []string{"ABP-001", "IPX-123"},
		},
		{
			name: "several sidecars decide nothing",
			sidecars: []pan.File{
				{ID: "nfo-1", Name: "ABP-001.nfo"},
				{ID: "nfo-2", Name: "ABP-002.nfo"},
			},
			videos: []scanVideo{{File: pan.File{ID: "v1", Name: "feature.mp4", Size: 1 << 30}}},
			want:   []string{""},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			library, client := panConcurrencyFixture(t)
			source := library.drive.snapshot().source()
			bodies := map[string][]byte{}
			if test.document != "" {
				body, err := nfo.Encode(nfo.Movie{Code: test.document, Title: "Fixture"})
				if err != nil {
					t.Fatal(err)
				}
				bodies["nfo"] = body
			}
			metadata := &scanMetadataClient{panClient: client, bodies: bodies}
			library.drive.client = metadata

			if err := library.resolveSingleNFO(t.Context(), source,
				library.drive.snapshot().authorizationVersion, test.sidecars, test.videos); err != nil {
				t.Fatal(err)
			}
			for index, want := range test.want {
				if test.videos[index].Code != want {
					t.Fatalf("video %d = %q, want %q", index, test.videos[index].Code, want)
				}
			}
			// The sidecar's own name usually states the number, and every read is
			// a request to 115.
			if metadata.reads != test.reads {
				t.Fatalf("NFO reads = %d, want %d", metadata.reads, test.reads)
			}
		})
	}
}

// The canonical number has to be in place before the movie row is written, or
// the library keeps a number JavDB cannot resolve and the scrape fails.
func TestScanIndexesTheNumberTheSidecarCarries(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	ctx := t.Context()
	source := library.drive.snapshot().source()
	body, err := nfo.Encode(nfo.Movie{Code: "GANA-3458", Title: "Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	entries := []pan.File{
		{ID: "video", ParentID: source.Directory.ID, Name: "4k688.com@200GANA-3458.mp4", Size: 1 << 30, SHA1: "video", PickCode: "video"},
		{ID: "nfo", ParentID: source.Directory.ID, Name: "GANA-3458.nfo", SHA1: "nfo", PickCode: "nfo"},
	}
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		return pan.FilePage{Files: entries, Total: len(entries), Path: []pan.Directory{{ID: source.Directory.ID}}}, nil
	}
	metadata := &scanMetadataClient{panClient: client, bodies: map[string][]byte{"nfo": body}}
	library.drive.client = metadata

	queued := library.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	if err := library.Scan(ctx, TaskJob{ID: queued.ID, Payload: queued.Payload}); err != nil {
		t.Fatal(err)
	}
	if metadata.reads != 0 {
		t.Fatalf("the sidecar's name states the number, so nothing had to be read: %d reads", metadata.reads)
	}
	film := library.database.Movie.Query().OnlyX(ctx)
	if film.Code != "GANA-3458" {
		t.Fatalf("indexed %q, want the sidecar's number", film.Code)
	}
	entry := library.database.File.Query().Where(file.FileIDEQ("video")).OnlyX(ctx)
	if valueOrZero(entry.MovieID) != film.ID {
		t.Fatalf("video was not bound to the movie: %#v", entry)
	}
}
