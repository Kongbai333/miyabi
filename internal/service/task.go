package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
)

type TaskInfo struct {
	ID            int           `json:"id"`
	Type          string        `json:"type"`
	Status        task.Status   `json:"status"`
	Progress      int           `json:"progress"`
	Error         *string       `json:"error,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Source        LibrarySource `json:"source"`
	Scan          ScanProgress  `json:"scan"`
	OfflineTaskID int           `json:"offline_task_id,omitempty"`
}

// TaskTypes are the kinds of work a worker pool runs, in the order the task
// centre lists them. Anything else is refused rather than ignored, so a typo
// cannot look like a pause that worked.
var TaskTypes = []string{"scan", "scrape", "cover", "frame"}

// taskPausedSetting records the kinds a pool may not claim. It outlives a
// restart, because a queue the user stopped must stay stopped: covering the
// whole library again is not a decision a restart gets to make.
const taskPausedSetting = "task.paused"

// TaskQueue reports one kind of work: how much is waiting, how much is running
// and whether the pools may claim more of it.
type TaskQueue struct {
	Type    string `json:"type"`
	Queued  int    `json:"queued"`
	Running int    `json:"running"`
	Paused  bool   `json:"paused"`
}

// TaskJob is internal execution input. API responses never expose raw payloads.
type TaskJob struct {
	ID      int
	Type    string
	Payload json.RawMessage
}

type TaskRevisions struct {
	Library uint64 `json:"library"`
	Offline uint64 `json:"offline"`
	History uint64 `json:"history"`
	Monitor uint64 `json:"monitor"`
}

type TaskService struct {
	revisions   TaskRevisions
	database    *ent.Client
	queue       contextLock   // Enqueue and claim wait until a new mount is published.
	pending     chan struct{} // Closed and replaced on every notification.
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
	paused      map[string]bool
}

func NewTaskService(database *ent.Client) *TaskService {
	return &TaskService{
		database: database, pending: make(chan struct{}),
		subscribers: make(map[chan struct{}]struct{}),
		paused:      make(map[string]bool),
	}
}

func (service *TaskService) enqueueScan(ctx context.Context, source LibrarySource) (TaskInfo, error) {
	if err := service.queue.Lock(ctx); err != nil {
		return TaskInfo{}, err
	}
	defer service.queue.Unlock()
	record, err := ensureScanTask(ctx, service.database.Task, source, task.StatusQueued, task.StatusRunning)
	if err != nil {
		return TaskInfo{}, err
	}
	service.Notify()
	return scanTaskInfo(record)
}

// A new mount may reuse a queued scan. A running scan captured the previous
// source version and will stop when that mount changes, even if its ID matches.
func ensureScanTask(ctx context.Context, tasks *ent.TaskClient, source LibrarySource, reusable ...task.Status) (*ent.Task, error) {
	record, err := tasks.Query().Where(
		task.TypeEQ("scan"), task.StatusIn(reusable...),
		func(selector *sql.Selector) {
			selector.Where(sql.And(
				sql.Not(sqljson.HasKey(task.FieldPayload, sqljson.Path("target_id"))),
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("source", "account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("source", "directory", "id")),
			))
		},
	).First(ctx)
	if err == nil {
		return record, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("find active scan: %w", err)
	}
	payload, err := encodeTaskPayload(scanPayload{
		Source: source, Scan: ScanProgress{Stage: "queued", CurrentPath: source.Directory.Path},
	})
	if err != nil {
		return nil, err
	}
	record, err = tasks.Create().SetType("scan").SetPayload(payload).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("queue library scan: %w", err)
	}
	return record, nil
}

// The UI shows one workflow per scan. Metadata and artwork jobs remain durable
// individual tasks while their progress is folded into that compact workflow.
func (service *TaskService) List(ctx context.Context) ([]TaskInfo, error) {
	records, err := service.database.Task.Query().Where(task.TypeEQ("scan")).
		Order(ent.Desc(task.FieldID)).Limit(20).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scan tasks: %w", err)
	}
	active, err := service.database.Task.Query().Where(task.TypeIn("scan", "scrape", "cover"),
		task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Select("CASE WHEN type = 'scan' THEN id ELSE json_extract(payload, '$.scan_task_id') END").Distinct()
		}).Select(task.FieldID).Ints(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active library tasks: %w", err)
	}
	ids := make(map[int]bool)
	for _, record := range records {
		ids[record.ID] = true
	}
	var missing []int
	for _, id := range active {
		if !ids[id] {
			missing = append(missing, id)
			ids[id] = true
		}
	}
	if len(missing) > 0 {
		parents, err := service.database.Task.Query().Where(task.IDIn(missing...)).All(ctx)
		if err != nil {
			return nil, err
		}
		records = append(records, parents...)
	}
	result, err := service.workflowInfos(ctx, records)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(result, func(a, b TaskInfo) int {
		aActive := a.Status == task.StatusQueued || a.Status == task.StatusRunning
		bActive := b.Status == task.StatusQueued || b.Status == task.StatusRunning
		if aActive != bActive {
			if aActive {
				return -1
			}
			return 1
		}
		if a.Status == task.StatusRunning && b.Status == task.StatusQueued {
			return -1
		}
		if a.Status == task.StatusQueued && b.Status == task.StatusRunning {
			return 1
		}
		return b.ID - a.ID
	})
	return result, nil
}

// Queues reports what each pool is holding. A paused kind keeps its queued
// tasks: the workers stop claiming them and whatever is already running
// finishes on its own.
func (service *TaskService) Queues(ctx context.Context) ([]TaskQueue, error) {
	var counts []taskQueueCount
	if err := service.database.Task.Query().
		Where(task.StatusIn(task.StatusQueued, task.StatusRunning)).
		GroupBy(task.FieldType, task.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &counts); err != nil {
		return nil, fmt.Errorf("count queued tasks: %w", err)
	}
	byType := make(map[string]TaskQueue, len(counts))
	for _, count := range counts {
		queue := byType[count.Type]
		queue.Type = count.Type
		if count.Status == string(task.StatusRunning) {
			queue.Running = count.Count
		} else {
			queue.Queued = count.Count
		}
		byType[count.Type] = queue
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	result := make([]TaskQueue, 0, len(TaskTypes))
	for _, name := range TaskTypes {
		queue := byType[name]
		queue.Type = name
		queue.Paused = service.paused[name]
		result = append(result, queue)
	}
	return result, nil
}

type taskQueueCount struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type metadataTaskGroup struct {
	ParentID  int         `json:"parent_id"`
	Count     int         `json:"count"`
	Type      string      `json:"type"`
	Status    task.Status `json:"status"`
	Error     *string     `json:"error"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// Return one row per parent/type/status, with its count and latest change.
// Keep large cover documents inside SQLite instead of decoding every child.
func (service *TaskService) metadataGroups(ctx context.Context, records []*ent.Task) ([]metadataTaskGroup, error) {
	children := sql.Table(task.Table)
	parents := make([]any, 0, len(records))
	for _, record := range records {
		parents = append(parents, record.ID)
	}
	parent := "json_extract(" + children.C(task.FieldPayload) + ", '$.scan_task_id')"
	partition := "PARTITION BY " + parent + ", " + children.C(task.FieldType) + ", " + children.C(task.FieldStatus)
	groups := sql.Select(
		children.C(task.FieldID), sql.As(parent, "parent_id"),
		sql.As("COUNT(*) OVER ("+partition+")", "count"),
		sql.As("ROW_NUMBER() OVER ("+partition+" ORDER BY "+children.C(task.FieldUpdatedAt)+" DESC, "+children.C(task.FieldID)+" DESC)", "position"),
	).From(children).Where(sql.And(sql.In(children.C(task.FieldType), "scrape", "cover"),
		sqljson.ValueIn(children.C(task.FieldPayload), parents, sqljson.Path("scan_task_id")))).As("metadata_groups")
	var result []metadataTaskGroup
	err := service.database.Task.Query().Where(func(s *sql.Selector) {
		s.Join(groups).On(s.C(task.FieldID), groups.C(task.FieldID))
		s.Where(sql.EQ(groups.C("position"), 1))
		s.Select(s.C(task.FieldType), s.C(task.FieldStatus), s.C(task.FieldError), s.C(task.FieldUpdatedAt), groups.C("parent_id"), groups.C("count"))
	}).Select(task.FieldID).Scan(ctx, &result)
	return result, err
}

func (service *TaskService) Info(ctx context.Context, id int) (TaskInfo, error) {
	record, err := service.database.Task.Get(ctx, id)
	if err != nil {
		return TaskInfo{}, err
	}
	infos, err := service.workflowInfos(ctx, []*ent.Task{record})
	if err != nil {
		return TaskInfo{}, err
	}
	return infos[0], nil
}

func (service *TaskService) workflowInfos(ctx context.Context, records []*ent.Task) ([]TaskInfo, error) {
	result := make([]TaskInfo, 0, len(records))
	if len(records) == 0 {
		return result, nil
	}
	children, err := service.metadataGroups(ctx, records)
	if err != nil {
		return nil, fmt.Errorf("read metadata workflow progress: %w", err)
	}
	byParent := make(map[int][]metadataTaskGroup)
	for _, child := range children {
		byParent[child.ParentID] = append(byParent[child.ParentID], child)
	}
	for _, record := range records {
		info, err := scanTaskInfo(record)
		if err != nil {
			return nil, err
		}
		active, running, failed, artwork := false, false, false, false
		for _, child := range byParent[record.ID] {
			if child.Type == "scrape" {
				info.Scan.MetadataTotal += child.Count
			}
			if (child.Type == "cover" && child.Status == task.StatusDone) || child.Status == task.StatusFailed {
				info.Scan.MetadataCompleted += child.Count
			}
			active = active || child.Status == task.StatusQueued || child.Status == task.StatusRunning
			running = running || child.Status == task.StatusRunning
			artwork = artwork || (child.Type == "cover" && child.Status == task.StatusRunning)
			if child.Status == task.StatusFailed {
				failed = true
				if info.Error == nil {
					info.Error = child.Error
				}
			}
			if child.UpdatedAt.After(info.UpdatedAt) {
				info.UpdatedAt = child.UpdatedAt
			}
		}
		if info.Scan.MetadataTotal > 0 && info.Status != task.StatusFailed {
			info.Progress = info.Scan.MetadataCompleted * 100 / info.Scan.MetadataTotal
			if active {
				info.Status, info.Scan.Stage = task.StatusQueued, "scraping"
				if running {
					info.Status = task.StatusRunning
				}
				if artwork {
					info.Scan.Stage = "artwork"
				}
			} else if failed {
				info.Status = task.StatusFailed
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func scanTaskInfo(record *ent.Task) (TaskInfo, error) {
	payload, err := decodeTaskPayload[scanPayload](record.Payload)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("read scan task %d: %w", record.ID, err)
	}
	return TaskInfo{
		ID: record.ID, Type: record.Type, Status: record.Status, Progress: record.Progress,
		Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		Source: payload.Source, Scan: payload.Scan, OfflineTaskID: payload.OfflineTaskID,
	}, nil
}

func (service *TaskService) Recover(ctx context.Context, types []string) error {
	if _, err := service.database.Task.Update().Where(
		task.TypeIn(types...), task.StatusEQ(task.StatusRunning),
	).SetStatus(task.StatusQueued).SetProgress(0).ClearError().Save(ctx); err != nil {
		return fmt.Errorf("recover interrupted tasks: %w", err)
	}
	service.Notify()
	return nil
}

// Pause stops the pools from claiming these kinds of work. A task that is
// already running is not interrupted, so a pause can let the current one finish
// while the queue behind it stands still.
func (service *TaskService) Pause(ctx context.Context, types []string) error {
	return service.setPaused(ctx, types, true)
}

// Resume lets the pools claim these kinds of work again.
func (service *TaskService) Resume(ctx context.Context, types []string) error {
	return service.setPaused(ctx, types, false)
}

func (service *TaskService) setPaused(ctx context.Context, types []string, paused bool) error {
	if err := validateTaskTypes(types); err != nil {
		return err
	}
	service.mu.Lock()
	for _, name := range types {
		if paused {
			service.paused[name] = true
			continue
		}
		delete(service.paused, name)
	}
	names := service.pausedTypes()
	service.mu.Unlock()
	if err := saveSetting(ctx, service.database, taskPausedSetting, names); err != nil {
		return err
	}
	// Waiting pools have to reconsider, and the notification redraws the queues
	// in the page that asked for the change.
	service.Notify()
	return nil
}

// pausedTypes lists what is paused, in the order the task centre shows it.
// Callers hold the lock.
func (service *TaskService) pausedTypes() []string {
	names := make([]string, 0, len(service.paused))
	for _, name := range TaskTypes {
		if service.paused[name] {
			names = append(names, name)
		}
	}
	return names
}

// Restore brings back the paused kinds a previous run left behind. Everything
// else starts clean: an interrupted scan is work to finish, but a queue the user
// stopped is a decision to keep.
func (service *TaskService) Restore(ctx context.Context) error {
	types, found, err := loadSetting[[]string](ctx, service.database, taskPausedSetting)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	for _, name := range types {
		// A stored value from an older build may name a kind this one does not run.
		if slices.Contains(TaskTypes, name) {
			service.paused[name] = true
		}
	}
	return nil
}

// CancelQueued drops the tasks of these kinds that have not started. A running
// task is left alone, because the work it is doing cannot be undone halfway.
func (service *TaskService) CancelQueued(ctx context.Context, types []string) (int, error) {
	if err := validateTaskTypes(types); err != nil {
		return 0, err
	}
	removed, err := service.database.Task.Delete().Where(
		task.TypeIn(types...), task.StatusEQ(task.StatusQueued),
	).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("cancel queued tasks: %w", err)
	}
	if removed > 0 {
		// A queued metadata task is what the library cards call "in progress".
		service.NotifyLibraryChanged()
	}
	return removed, nil
}

// claimable drops the paused kinds from what a pool may take.
func (service *TaskService) claimable(types []string) []string {
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.paused) == 0 {
		return types
	}
	ready := make([]string, 0, len(types))
	for _, name := range types {
		if !service.paused[name] {
			ready = append(ready, name)
		}
	}
	return ready
}

func validateTaskTypes(types []string) error {
	if len(types) == 0 {
		return domain.E(domain.KindInvalid, "缺少任务类型", nil)
	}
	for _, name := range types {
		if !slices.Contains(TaskTypes, name) {
			return domain.E(domain.KindInvalid, "未知的任务类型："+name, nil)
		}
	}
	return nil
}

// Claim takes the oldest waiting task of an unpaused kind. Work that is paused
// stays in the queue until a resume lets a worker take it.
func (service *TaskService) Claim(ctx context.Context, types []string) (*TaskJob, error) {
	if err := service.queue.Lock(ctx); err != nil {
		return nil, err
	}
	defer service.queue.Unlock()
	types = service.claimable(types)
	if len(types) == 0 {
		return nil, nil
	}
	for {
		record, err := service.database.Task.Query().Where(
			task.TypeIn(types...), task.StatusEQ(task.StatusQueued),
		).Order(ent.Asc(task.FieldID)).First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("find queued task: %w", err)
		}
		claimed, err := service.database.Task.Update().Where(
			task.IDEQ(record.ID), task.StatusEQ(task.StatusQueued),
		).SetStatus(task.StatusRunning).SetProgress(0).ClearError().Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("claim task %d: %w", record.ID, err)
		}
		if claimed == 0 {
			continue
		}
		service.Notify()
		return &TaskJob{ID: record.ID, Type: record.Type, Payload: record.Payload}, nil
	}
}

func (service *TaskService) Finish(ctx context.Context, id int, runError error) error {
	libraryChanged := false
	if err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		record, err := tx.Task.Get(ctx, id)
		if err != nil {
			return err
		}
		update := tx.Task.UpdateOneID(id)
		if runError != nil {
			update.SetStatus(task.StatusFailed).SetError(runError.Error())
			if record.Type == "scrape" || record.Type == "cover" {
				libraryChanged = true
				input, err := decodeTaskPayload[metadataPayload](record.Payload)
				if err != nil {
					return err
				}
				if err := tx.Movie.Update().Where(movie.IDEQ(input.MovieID), movie.ScrapeStatusNEQ(movie.ScrapeStatusDone),
					movie.HasFilesWith(libraryFiles(input.Source))).SetScrapeStatus(movie.ScrapeStatusFailed).Exec(ctx); err != nil {
					return err
				}
				// A failed scrape is exactly the case the card has no image for.
				if _, err := enqueueFrameTask(ctx, tx, input); err != nil {
					return err
				}
			}
		} else {
			update.SetStatus(task.StatusDone).SetProgress(100).ClearError()
		}
		return update.Exec(ctx)
	}); err != nil {
		return fmt.Errorf("finish task %d: %w", id, err)
	}
	if libraryChanged {
		service.NotifyLibraryChanged()
	} else {
		service.NotifyOfflineChanged()
	}
	return nil
}

// Pending reports the channel that closes when new work may be waiting. A pool
// reads it before claiming, so a task enqueued while its workers are busy closes
// the channel they hold rather than racing for a single wake-up token.
func (service *TaskService) Pending() <-chan struct{} {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.pending
}

// Notifications are coalesced. Each consumer reads a fresh database snapshot,
// so a slow SSE client cannot block workers or accumulate progress events.
func (service *TaskService) Subscribe() (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	service.mu.Lock()
	service.subscribers[updates] = struct{}{}
	service.mu.Unlock()
	return updates, func() {
		service.mu.Lock()
		delete(service.subscribers, updates)
		service.mu.Unlock()
	}
}

func (service *TaskService) Notify() {
	service.notify(false, false, false)
}

func (service *TaskService) NotifyLibraryChanged() {
	service.notify(true, false, true)
}

func (service *TaskService) NotifyOfflineChanged() {
	service.notify(false, true, false)
}

func (service *TaskService) NotifyWatchHistoryChanged() {
	service.notify(false, false, true)
}

// Monitor changes only refresh the watch list; they never wake the task pool.
func (service *TaskService) NotifyMonitorChanged() {
	service.mu.Lock()
	service.revisions.Monitor++
	subscribers := make([]chan struct{}, 0, len(service.subscribers))
	for subscriber := range service.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	service.mu.Unlock()
	for _, subscriber := range subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func (service *TaskService) Revisions() TaskRevisions {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.revisions
}

func (service *TaskService) notify(library, offline, history bool) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if !history || library || offline {
		// Closing the channel releases every waiting worker, where a buffered
		// send would reach only one of the pools. The replacement channel is
		// what the next wait reads.
		close(service.pending)
		service.pending = make(chan struct{})
	}
	if library {
		service.revisions.Library++
	}
	if offline {
		service.revisions.Offline++
	}
	if history {
		service.revisions.History++
	}
	for subscriber := range service.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// Keep the stored JSON shape while avoiding an intermediate map and its
// float64 conversion of task IDs. ent writes RawMessage as a JSON object.
func encodeTaskPayload(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode task payload: %w", err)
	}
	return encoded, nil
}

func decodeTaskPayload[T any](payload json.RawMessage) (T, error) {
	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, fmt.Errorf("decode stored task: %w", err)
	}
	return value, nil
}

// Partial progress updates retain unknown fields and copy nested values as JSON
// instead of decoding large documents that the update does not use.
func setTaskPayloadField(payload json.RawMessage, key string, value any) (json.RawMessage, error) {
	fields, err := decodeTaskPayload[map[string]json.RawMessage](payload)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("stored task payload must be an object")
	}
	encoded, err := encodeTaskPayload(value)
	if err != nil {
		return nil, err
	}
	fields[key] = encoded
	return encodeTaskPayload(fields)
}
