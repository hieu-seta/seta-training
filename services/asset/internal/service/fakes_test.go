package service_test

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/services/asset/internal/model"
)

type fakeFolderRepo struct {
	mu      sync.Mutex
	folders map[uuid.UUID]*model.Folder
	shares  map[uuid.UUID]map[uuid.UUID]string // folderID → uid → access
}

func newFakeFolderRepo() *fakeFolderRepo {
	return &fakeFolderRepo{folders: map[uuid.UUID]*model.Folder{}, shares: map[uuid.UUID]map[uuid.UUID]string{}}
}
func (r *fakeFolderRepo) Create(_ context.Context, f *model.Folder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.folders[f.ID] = f
	return nil
}
func (r *fakeFolderRepo) ByID(_ context.Context, id uuid.UUID) (*model.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.folders[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *f
	return &cp, nil
}
func (r *fakeFolderRepo) Update(_ context.Context, f *model.Folder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.folders[f.ID] = f
	return nil
}
func (r *fakeFolderRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.folders, id)
	return nil
}
func (r *fakeFolderRepo) ListVisible(_ context.Context, uid uuid.UUID, _ []uuid.UUID, _, _ int) ([]model.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []model.Folder{}
	for _, f := range r.folders {
		if f.OwnerID == uid {
			out = append(out, *f)
			continue
		}
		if r.shares[f.ID] != nil {
			if _, ok := r.shares[f.ID][uid]; ok {
				out = append(out, *f)
			}
		}
	}
	return out, nil
}
func (r *fakeFolderRepo) Share(_ context.Context, s *model.FolderShare) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shares[s.FolderID] == nil {
		r.shares[s.FolderID] = map[uuid.UUID]string{}
	}
	r.shares[s.FolderID][s.UserID] = s.Access
	return nil
}
func (r *fakeFolderRepo) Unshare(_ context.Context, folderID, uid uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shares[folderID] == nil || r.shares[folderID][uid] == "" {
		return httpx.ErrNotFound
	}
	delete(r.shares[folderID], uid)
	return nil
}
func (r *fakeFolderRepo) ShareAccess(_ context.Context, folderID, uid uuid.UUID) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shares[folderID] == nil {
		return "", nil
	}
	return r.shares[folderID][uid], nil
}

type fakeNoteRepo struct {
	mu         sync.Mutex
	notes      map[uuid.UUID]*model.Note
	noteShares map[uuid.UUID]map[uuid.UUID]string
	folderRepo *fakeFolderRepo // for inheritance lookup
}

func newFakeNoteRepo(fr *fakeFolderRepo) *fakeNoteRepo {
	return &fakeNoteRepo{notes: map[uuid.UUID]*model.Note{}, noteShares: map[uuid.UUID]map[uuid.UUID]string{}, folderRepo: fr}
}
func (r *fakeNoteRepo) Create(_ context.Context, n *model.Note) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes[n.ID] = n
	return nil
}
func (r *fakeNoteRepo) ByID(_ context.Context, id uuid.UUID) (*model.Note, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notes[id]
	if !ok {
		return nil, httpx.ErrNotFound
	}
	cp := *n
	return &cp, nil
}
func (r *fakeNoteRepo) Update(_ context.Context, n *model.Note) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes[n.ID] = n
	return nil
}
func (r *fakeNoteRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.notes, id)
	return nil
}
func (r *fakeNoteRepo) ListInFolder(_ context.Context, folderID uuid.UUID) ([]model.Note, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []model.Note{}
	for _, n := range r.notes {
		if n.FolderID == folderID {
			out = append(out, *n)
		}
	}
	return out, nil
}
func (r *fakeNoteRepo) Share(_ context.Context, s *model.NoteShare) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.noteShares[s.NoteID] == nil {
		r.noteShares[s.NoteID] = map[uuid.UUID]string{}
	}
	r.noteShares[s.NoteID][s.UserID] = s.Access
	return nil
}
func (r *fakeNoteRepo) Unshare(_ context.Context, noteID, uid uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.noteShares[noteID] == nil || r.noteShares[noteID][uid] == "" {
		return httpx.ErrNotFound
	}
	delete(r.noteShares[noteID], uid)
	return nil
}

// ShareAccess: union note shares + parent folder shares, write beats read.
func (r *fakeNoteRepo) ShareAccess(_ context.Context, noteID, uid uuid.UUID) (string, error) {
	r.mu.Lock()
	note, ok := r.notes[noteID]
	r.mu.Unlock()
	if !ok {
		return "", nil
	}
	r.mu.Lock()
	noteAccess := ""
	if r.noteShares[noteID] != nil {
		noteAccess = r.noteShares[noteID][uid]
	}
	r.mu.Unlock()
	folderAccess, _ := r.folderRepo.ShareAccess(context.Background(), note.FolderID, uid)
	return strongerAccess(noteAccess, folderAccess), nil
}

func strongerAccess(a, b string) string {
	if a == model.AccessWrite || b == model.AccessWrite {
		return model.AccessWrite
	}
	if a == model.AccessRead || b == model.AccessRead {
		return model.AccessRead
	}
	return ""
}

type fakeTeamClient struct {
	mgrsOf map[uuid.UUID][]uuid.UUID
	err    error
}

func newFakeTeamClient() *fakeTeamClient { return &fakeTeamClient{mgrsOf: map[uuid.UUID][]uuid.UUID{}} }

func (t *fakeTeamClient) ManagersOf(_ context.Context, uid uuid.UUID) ([]uuid.UUID, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.mgrsOf[uid], nil
}
