package service

import (
	"context"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

// The magnet page copies a full magnet URI, and the download form accepts a
// bare hash, so both have to reach 115 as a normalized URI.
func TestAddMagnetSubmitsAMagnetTheCatalogueNeverSaw(t *testing.T) {
	service, client := offlineAddFixture(t)
	submitted := ""
	client.addOffline = func(_ context.Context, _, uri, _ string) (string, error) {
		submitted = uri
		return strings.TrimPrefix(uri, "magnet:?xt=urn:btih:"), nil
	}

	submission, err := service.AddMagnet(t.Context(), "magnet:?xt=urn:btih:"+strings.ToUpper(offlineHashA)+"&dn=ABP-001")
	if err != nil {
		t.Fatal(err)
	}
	if submitted != "magnet:?xt=urn:btih:"+offlineHashA {
		t.Fatalf("submitted %q", submitted)
	}
	if submission.Hash != offlineHashA || submission.TaskID == 0 {
		t.Fatalf("submission = %+v", submission)
	}

	if _, err := service.AddMagnet(t.Context(), strings.ToUpper(offlineHashB)); err != nil {
		t.Fatal(err)
	}
	if submitted != "magnet:?xt=urn:btih:"+offlineHashB {
		t.Fatalf("submitted %q", submitted)
	}

	// Neither task claims a catalogue movie, which is what lets the download
	// finish without a targeted scan.
	records := service.database.Task.Query().Where(task.TypeEQ("offline")).AllX(t.Context())
	if len(records) != 2 {
		t.Fatalf("offline tasks = %d", len(records))
	}
	for _, record := range records {
		input, err := decodeTaskPayload[offlinePayload](record.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if input.Code != "" || input.JavDBID != "" {
			t.Fatalf("an unknown magnet carried catalogue identity: %+v", input)
		}
	}
}

func TestAddMagnetRejectsALinkWithoutAnInfoHash(t *testing.T) {
	// No drive, no discovery, no database: an unusable link must be refused
	// before any of them is touched.
	service := &OfflineService{}
	for _, input := range []string{"", "   ", "magnet:?xt=urn:btih:nothex", "https://example.com/x.torrent"} {
		if _, err := service.AddMagnet(t.Context(), input); err == nil {
			t.Fatalf("%q was accepted", input)
		}
	}
}

func TestOfflineCompletionSkipsTheTargetedScanForAnUnknownMagnet(t *testing.T) {
	service, record, input, _ := offlineFixture(t)
	ctx := t.Context()
	input.Code, input.JavDBID = "", ""
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	record = service.database.Task.UpdateOne(record).SetPayload(encoded).SaveX(ctx)

	before := service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(ctx)
	if err := service.updateTask(ctx, record, pan.OfflineTask{Status: 2, FileID: "download-folder"}, service.drive.snapshot()); err != nil {
		t.Fatal(err)
	}
	if after := service.database.Task.Query().Where(task.TypeEQ("scan")).CountX(ctx); after != before {
		t.Fatalf("an unknown magnet queued %d targeted scans", after-before)
	}

	done := service.database.Task.GetX(ctx, record.ID)
	if done.Status != task.StatusDone {
		t.Fatalf("status = %v", done.Status)
	}
	saved, err := decodeTaskPayload[offlinePayload](done.Payload)
	if err != nil {
		t.Fatal(err)
	}
	// The completion still recorded the download; only the scan is skipped.
	if saved.ScanTaskID != 0 || saved.FileID != "download-folder" {
		t.Fatalf("an unknown magnet's completion = %+v", saved)
	}
}
