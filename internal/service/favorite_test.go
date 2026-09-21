package service

import (
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

func favoriteFixture(t *testing.T) (*LibraryService, int, int, int) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	service := NewLibraryService(store.Client, nil, NewTaskService(store.Client), nil)
	// Media list endpoints read the mounted directory, so the fixture mounts one.
	if err := saveSetting(ctx, store.Client, panDirectorySetting, panLibraryDirectory{
		AccountID: "100", PanLibraryDirectory: PanLibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}); err != nil {
		t.Fatal(err)
	}
	first := store.Client.Movie.Create().SetCode("ABP-001").SetTitle("First").SaveX(ctx)
	second := store.Client.Movie.Create().SetCode("ABP-002").SetTitle("Second").SaveX(ctx)
	for index, id := range []int{first.ID, second.ID} {
		store.Client.File.Create().SetFileID(string(rune('a' + index))).SetName("video.mp4").
			SetSize(1).SetAccountID("100").SetRootID("10").SetMovieID(id).SaveX(ctx)
	}
	return service, first.ID, second.ID, 0
}

func TestFavoriteGroupsCreateRenameDeleteAndRejectDuplicates(t *testing.T) {
	service, _, _, _ := favoriteFixture(t)
	ctx := t.Context()

	watched, err := service.CreateFavoriteGroup(ctx, " 想看 ")
	if err != nil || watched.Name != "想看" {
		t.Fatalf("create = %+v, %v", watched, err)
	}
	if _, err := service.CreateFavoriteGroup(ctx, "想看"); !errors.Is(err, ErrFavoriteGroupUsed) {
		t.Fatalf("duplicate name error = %v", err)
	}
	if _, err := service.CreateFavoriteGroup(ctx, "   "); !errors.Is(err, ErrFavoriteGroupName) {
		t.Fatalf("blank name error = %v", err)
	}
	if _, err := service.CreateFavoriteGroup(ctx, string(make([]rune, favoriteNameLimit+1))); err == nil {
		t.Fatal("overlong name was accepted")
	}
	// The limit counts characters, so a full-length Chinese name must pass.
	if _, err := service.CreateFavoriteGroup(ctx, "收藏分组名称测试一二三四五六七八九十"); err != nil {
		t.Fatalf("chinese name within the limit was rejected: %v", err)
	}

	renamed, err := service.RenameFavoriteGroup(ctx, watched.ID, "稍后看")
	if err != nil || renamed.Name != "稍后看" {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	if _, err := service.RenameFavoriteGroup(ctx, 9999, "任意"); !errors.Is(err, ErrFavoriteGroupGone) {
		t.Fatalf("missing group error = %v", err)
	}
	if err := service.DeleteFavoriteGroup(ctx, watched.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteFavoriteGroup(ctx, watched.ID); !errors.Is(err, ErrFavoriteGroupGone) {
		t.Fatalf("double delete error = %v", err)
	}
}

func TestFavoriteSetReplacesMembershipAndFiltersTheList(t *testing.T) {
	service, first, second, _ := favoriteFixture(t)
	ctx := t.Context()
	firstGroup, err := service.CreateFavoriteGroup(ctx, "喜欢")
	if err != nil {
		t.Fatal(err)
	}
	secondGroup, err := service.CreateFavoriteGroup(ctx, "已看")
	if err != nil {
		t.Fatal(err)
	}

	// A movie may sit in several groups at once.
	if err := service.SetFavorite(ctx, first, []int{firstGroup.ID, secondGroup.ID}); err != nil {
		t.Fatal(err)
	}
	// A repeated id must not violate the pair index.
	if err := service.SetFavorite(ctx, second, []int{firstGroup.ID, firstGroup.ID}); err != nil {
		t.Fatal(err)
	}

	groups, err := service.FavoriteGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Count != 2 || groups[1].Count != 1 {
		t.Fatalf("group counts = %+v", groups)
	}

	page, err := service.Movies(ctx, 1, 20, LibraryFilter{GroupIDs: []int{firstGroup.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Movies) != 2 {
		t.Fatalf("filtered page = %+v", page)
	}
	page, err = service.Movies(ctx, 1, 20, LibraryFilter{GroupIDs: []int{secondGroup.ID}})
	if err != nil || page.Total != 1 || page.Movies[0].ID != first {
		t.Fatalf("second group page = %+v, %v", page, err)
	}
	if !equalInts(page.Movies[0].FavoriteGroupIDs, []int{firstGroup.ID, secondGroup.ID}) {
		t.Fatalf("card groups = %v", page.Movies[0].FavoriteGroupIDs)
	}

	// Setting a new set replaces the old membership instead of appending.
	if err := service.SetFavorite(ctx, first, []int{secondGroup.ID}); err != nil {
		t.Fatal(err)
	}
	page, err = service.Movies(ctx, 1, 20, LibraryFilter{GroupIDs: []int{firstGroup.ID}})
	if err != nil || page.Total != 1 || page.Movies[0].ID != second {
		t.Fatalf("stale membership survived: %+v, %v", page, err)
	}

	// An empty set clears the star.
	if err := service.SetFavorite(ctx, second, nil); err != nil {
		t.Fatal(err)
	}
	page, err = service.Movies(ctx, 1, 20, LibraryFilter{})
	if err != nil || page.Total != 2 {
		t.Fatalf("clearing a star changed the library: %+v, %v", page, err)
	}
	for _, item := range page.Movies {
		if len(item.FavoriteGroupIDs) != 0 && item.ID == second {
			t.Fatalf("star was not cleared: %+v", item)
		}
	}
}

func TestFavoriteRejectsUnknownTargetsAndCascadesOnGroupDelete(t *testing.T) {
	service, first, _, _ := favoriteFixture(t)
	ctx := t.Context()
	group, err := service.CreateFavoriteGroup(ctx, "临时")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetFavorite(ctx, first, []int{group.ID}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetFavorite(ctx, 9999, []int{group.ID}); !errors.Is(err, ErrFavoriteMovieGone) {
		t.Fatalf("missing movie error = %v", err)
	}
	if err := service.SetFavorite(ctx, first, []int{9999}); !errors.Is(err, ErrFavoriteGroupGone) {
		t.Fatalf("missing group error = %v", err)
	}
	if err := service.SetFavorite(ctx, first, []int{0}); err == nil {
		t.Fatal("zero group id was accepted")
	}

	// Deleting a group drops its stars but keeps the movie in the library.
	if err := service.DeleteFavoriteGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	page, err := service.Movies(ctx, 1, 20, LibraryFilter{})
	if err != nil || page.Total != 2 {
		t.Fatalf("group delete removed movies: %+v, %v", page, err)
	}
	for _, item := range page.Movies {
		if item.ID == first && len(item.FavoriteGroupIDs) != 0 {
			t.Fatalf("cascade left a dangling star: %+v", item)
		}
	}
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
