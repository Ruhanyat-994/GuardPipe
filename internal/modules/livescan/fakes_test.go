package livescan_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// --- hand-written fakes (no mocking framework) ---

type fakeRepo struct {
	mu        sync.Mutex
	rows      map[uuid.UUID]livescan.Webhook
	createErr error
	statuses  []string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[uuid.UUID]livescan.Webhook{}} }

func (f *fakeRepo) Create(_ context.Context, w *livescan.Webhook) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	f.rows[w.ID] = *w
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*livescan.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.rows[id]
	if !ok {
		return nil, apperrors.NotFound("webhook.not_found", "not found")
	}
	return &w, nil
}

func (f *fakeRepo) GetByProjectID(_ context.Context, projectID uuid.UUID) (*livescan.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.rows {
		if w.ProjectID == projectID {
			return &w, nil
		}
	}
	return nil, apperrors.NotFound("webhook.not_found", "not found")
}

func (f *fakeRepo) UpdateConfig(_ context.Context, w *livescan.Webhook) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[w.ID] = *w
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, id)
	return nil
}

func (f *fakeRepo) RecordDelivery(_ context.Context, id uuid.UUID, at time.Time, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.rows[id]
	w.LastDeliveryAt, w.LastDeliveryStatus = &at, status
	f.rows[id] = w
	f.statuses = append(f.statuses, status)
	return nil
}

func (f *fakeRepo) Pause(_ context.Context, id uuid.UUID, reason string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.rows[id]
	w.PausedReason, w.PausedAt = &reason, &at
	f.rows[id] = w
	return nil
}

func (f *fakeRepo) only() livescan.Webhook {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.rows {
		return w
	}
	return livescan.Webhook{}
}

type createdHook struct {
	owner, name, token, url, secret string
	events                          []string
}

type fakeHooks struct {
	created   []createdHook
	deleted   []int64
	createErr error
	deleteErr error
}

func (f *fakeHooks) CreateHook(_ context.Context, owner, name, token, url, secret string, events []string) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.created = append(f.created, createdHook{owner, name, token, url, secret, events})
	return 777, nil
}

func (f *fakeHooks) DeleteHook(_ context.Context, _, _, _ string, hookID int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, hookID)
	return nil
}

// fakeProjects has one project in orgID with a repository attached.
type fakeProjects struct {
	projectID     uuid.UUID
	orgID         uuid.UUID
	repoURL       string
	defaultBranch string
	token         string
	noRepository  bool
}

func (f *fakeProjects) Get(_ context.Context, actor domain.Actor, id uuid.UUID) (*project.ProjectDetail, error) {
	if id != f.projectID || actor.OrgID != f.orgID {
		return nil, apperrors.NotFound("project.not_found", "project not found")
	}
	d := &project.ProjectDetail{Project: project.Project{ID: id, OrgID: f.orgID}}
	if !f.noRepository {
		d.Repository = &project.Repository{URL: f.repoURL, DefaultBranch: f.defaultBranch}
	}
	return d, nil
}

func (f *fakeProjects) GetCloneInfo(_ context.Context, id uuid.UUID) (string, string, string, error) {
	if id != f.projectID || f.noRepository {
		return "", "", "", apperrors.NotFound("project.repository_not_found", "no repository")
	}
	return f.repoURL, f.defaultBranch, f.token, nil
}

func (f *fakeProjects) GetOrgID(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	if id != f.projectID {
		return uuid.Nil, apperrors.NotFound("project.not_found", "project not found")
	}
	return f.orgID, nil
}

type createdScan struct {
	actor     domain.Actor
	projectID uuid.UUID
	in        orchestrator.CreateScanInput
}

type fakeScans struct {
	calls []createdScan
	err   error
}

func (f *fakeScans) CreateScan(_ context.Context, actor domain.Actor, projectID uuid.UUID, in orchestrator.CreateScanInput) (*orchestrator.ScanDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, createdScan{actor, projectID, in})
	return &orchestrator.ScanDetail{Scan: domain.Scan{ID: uuid.New(), ProjectID: projectID}}, nil
}

// fakeCoord mirrors adapters/queue.LiveScanStore's semantics in memory.
type fakeCoord struct {
	seen     map[string]bool
	events   [][]byte
	pending  map[string][]byte
	fireAt   map[string]time.Time
	counters map[string]int64
}

func newFakeCoord() *fakeCoord {
	return &fakeCoord{seen: map[string]bool{}, pending: map[string][]byte{}, fireAt: map[string]time.Time{}, counters: map[string]int64{}}
}

func (f *fakeCoord) MarkDeliverySeen(_ context.Context, id string, _ time.Duration) (bool, error) {
	if f.seen[id] {
		return false, nil
	}
	f.seen[id] = true
	return true, nil
}

func (f *fakeCoord) PushEvent(_ context.Context, p []byte) error {
	f.events = append(f.events, p)
	return nil
}

func (f *fakeCoord) PopEvent(context.Context, time.Duration) ([]byte, error) {
	if len(f.events) == 0 {
		return nil, nil
	}
	p := f.events[0]
	f.events = f.events[1:]
	return p, nil
}

func (f *fakeCoord) ArmPending(_ context.Context, key string, p []byte, fireAt time.Time) error {
	f.pending[key] = p
	if _, ok := f.fireAt[key]; !ok {
		f.fireAt[key] = fireAt
	}
	return nil
}

func (f *fakeCoord) PopDuePending(_ context.Context, now time.Time) ([][]byte, error) {
	var keys []string
	for k, at := range f.fireAt {
		if !at.After(now) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out [][]byte
	for _, k := range keys {
		out = append(out, f.pending[k])
		delete(f.pending, k)
		delete(f.fireAt, k)
	}
	return out, nil
}

func (f *fakeCoord) IncrWindowCount(_ context.Context, key string, _ time.Duration) (int64, error) {
	f.counters[key]++
	return f.counters[key], nil
}

type fakeAudit struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (f *fakeAudit) Log(_ context.Context, e audit.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

func (f *fakeAudit) List(context.Context, audit.ListFilter, audit.Page) ([]audit.Entry, int, error) {
	return nil, 0, nil
}

func (f *fakeAudit) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.entries))
	for i, e := range f.entries {
		out[i] = e.Action
	}
	return out
}

func (f *fakeAudit) last(action string) *audit.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.entries) - 1; i >= 0; i-- {
		if f.entries[i].Action == action {
			return &f.entries[i]
		}
	}
	return nil
}
