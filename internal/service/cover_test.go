package service

import (
	"testing"

	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestFindDirectoryNFOMatchesCatalogueAlias(t *testing.T) {
	directory := movieDirectory{Shared: true, Files: []pan.File{
		{ID: "different", Name: "LUXU-1099.nfo"},
		{ID: "alias", Name: "259LUXU-1899.nfo"},
	}}
	entry, found := findDirectoryNFO("LUXU-1899", directory)
	if !found || entry.ID != "alias" {
		t.Fatalf("matching alias NFO was not found: %#v, %v", entry, found)
	}
}

func TestDirectoryNFOSelectionPrefersExactFileOverAliasesAndFolders(t *testing.T) {
	directory := movieDirectory{Shared: true, Files: []pan.File{
		{ID: "folder", Name: "LUXU-1899.nfo", IsDirectory: true},
		{ID: "alias", Name: "259LUXU-1899.nfo"},
		{ID: "exact", Name: "luxu-1899.NFO"},
	}}
	entry, found := findDirectoryNFO("LUXU-1899", directory)
	if !found || entry.ID != "exact" {
		t.Fatalf("NFO selection = %+v, found=%t", entry, found)
	}
}

// The server-side alias table cannot cover every distributor, so a sidecar that
// spells the number differently must still be found.
func TestFindDirectoryNFOMatchesADistributorSpelling(t *testing.T) {
	directory := movieDirectory{Shared: true, Files: []pan.File{
		{ID: "different", Name: "GANA-3459.nfo"},
		{ID: "distributor", Name: "200GANA-3458.nfo"},
	}}
	entry, found := findDirectoryNFO("GANA-3458", directory)
	if !found || entry.ID != "distributor" {
		t.Fatalf("distributor NFO was not found: %#v, %v", entry, found)
	}
}

// The NFO of a single-film folder may spell the number the distributor's way,
// so only a number that names a different film is a conflict.
func TestDirectoryNFOToleratesADistributorSpelling(t *testing.T) {
	for _, test := range []struct {
		name     string
		code     string
		sidecar  string
		document string
		wantCode string
		conflict bool
	}{
		{name: "distributor prefix", code: "200GANA-3458", sidecar: "200GANA-3458.nfo", document: "GANA-3458", wantCode: "GANA-3458"},
		{name: "studio date prefix", code: "CARIB-060326-001", sidecar: "CARIB-060326-001.nfo", document: "060326-001", wantCode: "060326-001"},
		{name: "sidecar spelling", code: "GANA-3458", sidecar: "200GANA-3458.nfo", document: "GANA-3458", wantCode: "GANA-3458"},
		{name: "different number", code: "ABP-001", sidecar: "ABP-001.nfo", document: "IPX-123", conflict: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			library, client := panConcurrencyFixture(t)
			source := library.drive.snapshot().source()
			body, err := nfo.Encode(nfo.Movie{Code: test.document, Title: "Fixture"})
			if err != nil {
				t.Fatal(err)
			}
			library.drive.client = &scanMetadataClient{panClient: client, bodies: map[string][]byte{"nfo": body}}
			directory := movieDirectory{Files: []pan.File{
				{ID: "nfo", ParentID: source.Directory.ID, Name: test.sidecar, PickCode: "nfo"},
				{ID: "poster", ParentID: source.Directory.ID, Name: "poster.jpg"},
				{ID: "fanart", ParentID: source.Directory.ID, Name: "fanart.jpg"},
			}}
			input := metadataPayload{Source: source, Code: test.code}
			scrape := NewScrapeService(library, nil, library.images)
			doc, _, found, err := scrape.directoryNFO(t.Context(), input, library.drive.snapshot().authorizationVersion, directory)
			if (err != nil) != test.conflict {
				t.Fatalf("conflict = %v, error = %v", test.conflict, err)
			}
			if test.conflict {
				return
			}
			if !found || doc.Code != test.wantCode {
				t.Fatalf("NFO document = %#v, found = %v", doc, found)
			}
		})
	}
}

func TestCoverRetryPreservesEditsAndRecognizesItsOwnWriteback(t *testing.T) {
	poster, fanart := []byte("fixture poster"), []byte("fixture fanart")
	origin := artworkOrigin{
		Poster: pan.File{ParentID: "10", Name: "poster.jpg", SHA1: pan.SHA1(poster)},
		Fanart: pan.File{ParentID: "10", Name: "fanart.jpg", SHA1: pan.SHA1(fanart)},
	}
	doc := nfo.Movie{Code: "ABP-001", Title: "Fixture title"}
	current := doc
	current.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: "poster.jpg"}}
	current.Fanart = "fanart.jpg"
	if err := verifyCoverOrigin(coverPayload{Document: doc}, "10", current, origin, poster, fanart); err != nil {
		t.Fatalf("own writeback was rejected: %v", err)
	}
	input := coverPayload{Document: current, Origin: &origin}
	if err := verifyCoverOrigin(input, "10", current, origin, poster, fanart); err != nil {
		t.Fatalf("unchanged NFO was rejected: %v", err)
	}
	edited := current
	edited.Title = "User edit"
	if err := verifyCoverOrigin(input, "10", edited, origin, poster, fanart); err == nil {
		t.Fatal("edited NFO would be marked synchronized with old metadata")
	}
	changed := origin
	changed.Poster.SHA1 = pan.SHA1([]byte("edited poster"))
	if err := verifyCoverOrigin(input, "10", current, changed, poster, fanart); err == nil {
		t.Fatal("edited artwork would be marked synchronized with old cache")
	}
	if err := verifyCoverOrigin(coverPayload{Document: doc}, "10", edited, origin, poster, fanart); err == nil {
		t.Fatal("new user NFO was mistaken for an interrupted writeback")
	}
}
