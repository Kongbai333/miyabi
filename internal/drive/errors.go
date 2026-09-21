package drive

import "github.com/ppxb/miyabi/internal/domain"

var (
	// ErrSourceChanged indicates the mounted media directory or authenticated 115 account has changed.
	ErrSourceChanged = domain.E(domain.KindConflict, "媒体目录或登录账号已变更，请重新扫描", nil)

	// ErrMediaDirectoryRequired indicates operations requiring a mounted directory were attempted without one.
	ErrMediaDirectoryRequired = domain.E(domain.KindInvalid, "请先在设置页挂载当前 115 账号的媒体目录", nil)

	// ErrDirectoryIncomplete indicates directory listing changed or returned partial pages during traversal.
	ErrDirectoryIncomplete = domain.E(domain.KindConflict, "115 目录内容在读取期间变化或分页不完整，请重新扫描", nil)
)
