package drive

import (
	"context"

	"github.com/ppxb/miyabi/internal/pan"
)

// WalkFilePages traverses all pages of a 115 directory. A visitor returns false
// when its search is complete. Each page is validated before visiting so incomplete
// listings cannot commit a scan or authorize removing offline history.
func WalkFilePages(ctx context.Context, fetch func(offset int) (pan.FilePage, error), visit func(pan.FilePage) (bool, error)) error {
	total, offset := -1, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := fetch(offset)
		if err != nil {
			return err
		}
		if total == -1 {
			total = page.Total
		}
		offset += len(page.Files)
		if page.Total != total || total < 0 || offset > total ||
			page.HasMore && len(page.Files) == 0 || !page.HasMore && offset != total {
			return ErrDirectoryIncomplete
		}
		more, err := visit(page)
		if err != nil {
			return err
		}
		if !more || !page.HasMore {
			return nil
		}
	}
}

// WalkOfflinePages traverses pages of 115 offline tasks until exhausted or visitor returns false.
func WalkOfflinePages(ctx context.Context, fetch func(page int) (pan.OfflinePage, error), visit func(pan.OfflinePage) (bool, error)) error {
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		remote, err := fetch(page)
		if err != nil {
			return err
		}
		more, err := visit(remote)
		if err != nil {
			return err
		}
		if !more || page >= remote.PageCount {
			return nil
		}
	}
}
