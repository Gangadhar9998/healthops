package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeServerRepository struct {
	servers     map[string]RemoteServer
	listErr     error
	getErr      error
	createErr   error
	updateErr   error
	deleteErr   error
	createCalls int
	updateCalls int
	deleteCalls int
	lastCreated RemoteServer
	lastUpdated RemoteServer
	lastDeleted string
}

func newFakeServerRepository(servers ...RemoteServer) *fakeServerRepository {
	items := make(map[string]RemoteServer, len(servers))
	for _, srv := range servers {
		items[srv.ID] = srv
	}
	return &fakeServerRepository{servers: items}
}

func (f *fakeServerRepository) List(context.Context) ([]RemoteServer, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]RemoteServer, 0, len(f.servers))
	for _, srv := range f.servers {
		out = append(out, srv)
	}
	return out, nil
}

func (f *fakeServerRepository) Get(_ context.Context, id string) (RemoteServer, error) {
	if f.getErr != nil {
		return RemoteServer{}, f.getErr
	}
	srv, ok := f.servers[id]
	if !ok {
		return RemoteServer{}, ErrServerNotFound
	}
	return srv, nil
}

func (f *fakeServerRepository) Create(_ context.Context, srv RemoteServer) (RemoteServer, error) {
	if f.createErr != nil {
		return RemoteServer{}, f.createErr
	}
	if _, exists := f.servers[srv.ID]; exists {
		return RemoteServer{}, ErrServerAlreadyExists
	}
	f.createCalls++
	f.lastCreated = srv
	f.servers[srv.ID] = srv
	return srv, nil
}

func (f *fakeServerRepository) Update(_ context.Context, srv RemoteServer) (RemoteServer, error) {
	if f.updateErr != nil {
		return RemoteServer{}, f.updateErr
	}
	if _, exists := f.servers[srv.ID]; !exists {
		return RemoteServer{}, ErrServerNotFound
	}
	f.updateCalls++
	f.lastUpdated = srv
	f.servers[srv.ID] = srv
	return srv, nil
}

func (f *fakeServerRepository) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, exists := f.servers[id]; !exists {
		return ErrServerNotFound
	}
	f.deleteCalls++
	f.lastDeleted = id
	delete(f.servers, id)
	return nil
}

func (f *fakeServerRepository) SeedIfEmpty(context.Context, []RemoteServer) error {
	return nil
}

func TestHandleServersGetReadsRepositoryAndSyncsCache(t *testing.T) {
	repo := newFakeServerRepository(RemoteServer{
		ID:       "repo-1",
		Name:     "Repo Server",
		Host:     "10.0.0.1",
		User:     "ops",
		Password: "secret",
	})
	service := NewService(&Config{
		Servers: []RemoteServer{{
			ID:   "cached-1",
			Name: "Cached Server",
			Host: "10.0.0.2",
			User: "cached",
		}},
	}, &fakeStore{}, nil)
	service.SetServerRepo(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rec := httptest.NewRecorder()

	service.handleServers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var servers []RemoteServer
	if err := decodeAPIResponseData(rec.Body.Bytes(), &servers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(servers) != 1 || servers[0].ID != "repo-1" {
		t.Fatalf("unexpected servers payload: %+v", servers)
	}
	if servers[0].Password != "********" {
		t.Fatalf("expected password to be masked, got %q", servers[0].Password)
	}

	if len(service.cfg.Servers) != 1 || service.cfg.Servers[0].ID != "repo-1" {
		t.Fatalf("expected cache to sync from repository, got %+v", service.cfg.Servers)
	}
	if service.cfg.Servers[0].Password != "secret" {
		t.Fatalf("expected cache to keep stored password, got %q", service.cfg.Servers[0].Password)
	}
}

func TestHandleServersGetFallsBackToCacheWhenRepositoryReadFails(t *testing.T) {
	service := NewService(&Config{
		Servers: []RemoteServer{{
			ID:   "cached-1",
			Name: "Cached Server",
			Host: "10.0.0.2",
			User: "cached",
		}},
	}, &fakeStore{}, nil)
	service.SetServerRepo(&fakeServerRepository{
		listErr: &ServerRepositoryError{Op: "list", Err: ErrServerRepositoryNotConfigured, Cause: errors.New("repository unavailable")},
		servers: map[string]RemoteServer{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rec := httptest.NewRecorder()

	service.handleServers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var servers []RemoteServer
	if err := decodeAPIResponseData(rec.Body.Bytes(), &servers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(servers) != 1 || servers[0].ID != "cached-1" {
		t.Fatalf("expected cached server fallback, got %+v", servers)
	}
}

func TestHandleServersPostPersistsRepositoryAndSyncsCache(t *testing.T) {
	repo := newFakeServerRepository()
	service := NewService(&Config{}, &fakeStore{}, nil)
	service.SetServerRepo(repo)

	body := mustMarshalJSON(t, RemoteServer{
		ID:       "srv-1",
		Name:     "Production",
		Host:     "13.233.171.43",
		User:     "sai",
		Password: "secret",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleServers(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.createCalls != 1 {
		t.Fatalf("expected repository create to be called once, got %d", repo.createCalls)
	}
	if repo.lastCreated.ID != "srv-1" {
		t.Fatalf("expected created server to be persisted, got %+v", repo.lastCreated)
	}
	if len(service.cfg.Servers) != 1 || service.cfg.Servers[0].ID != "srv-1" {
		t.Fatalf("expected cfg.Servers cache to sync, got %+v", service.cfg.Servers)
	}
}

func TestHandleServerByIDPutPersistsRepositoryAndPreservesMaskedPassword(t *testing.T) {
	repo := newFakeServerRepository(RemoteServer{
		ID:       "srv-1",
		Name:     "Validation",
		Host:     "13.127.106.100",
		User:     "sai",
		Password: "secret",
	})
	service := NewService(&Config{}, &fakeStore{}, nil)
	service.SetServerRepo(repo)

	body := mustMarshalJSON(t, RemoteServer{
		Name:     "Validation Updated",
		Host:     "13.127.106.100",
		User:     "sai",
		Password: "********",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/servers/srv-1", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleServerByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.updateCalls != 1 {
		t.Fatalf("expected repository update to be called once, got %d", repo.updateCalls)
	}
	if repo.lastUpdated.Password != "secret" {
		t.Fatalf("expected masked password to preserve stored secret, got %q", repo.lastUpdated.Password)
	}
	if len(service.cfg.Servers) != 1 || service.cfg.Servers[0].Name != "Validation Updated" {
		t.Fatalf("expected cfg.Servers cache to sync, got %+v", service.cfg.Servers)
	}
	if service.cfg.Servers[0].Password != "secret" {
		t.Fatalf("expected cache to keep stored password, got %q", service.cfg.Servers[0].Password)
	}
}

func TestHandleServerByIDDeleteUsesStoreSnapshotChecks(t *testing.T) {
	repo := newFakeServerRepository(RemoteServer{
		ID:   "srv-1",
		Name: "Validation",
		Host: "13.127.106.100",
		User: "sai",
	})
	store := &fakeStore{
		snapshot: State{
			Checks: []CheckConfig{{
				ID:       "check-1",
				Name:     "SSH Health",
				Type:     "ssh",
				ServerId: "srv-1",
			}},
		},
	}
	service := NewService(&Config{}, store, nil)
	service.SetServerRepo(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/srv-1", nil)
	rec := httptest.NewRecorder()

	service.handleServerByID(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("expected repository delete not to be called, got %d", repo.deleteCalls)
	}
}

func TestHandleServerByIDDeletePersistsRepositoryAndUpdatesCache(t *testing.T) {
	repo := newFakeServerRepository(RemoteServer{
		ID:   "srv-1",
		Name: "Production",
		Host: "13.233.171.43",
		User: "sai",
	})
	service := NewService(&Config{
		Servers: []RemoteServer{{
			ID:   "srv-1",
			Name: "Cached Production",
			Host: "127.0.0.1",
			User: "cached",
		}},
	}, &fakeStore{}, nil)
	service.SetServerRepo(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/servers/srv-1", nil)
	rec := httptest.NewRecorder()

	service.handleServerByID(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.deleteCalls != 1 || repo.lastDeleted != "srv-1" {
		t.Fatalf("expected repository delete for srv-1, got calls=%d last=%q", repo.deleteCalls, repo.lastDeleted)
	}
	if len(service.cfg.Servers) != 0 {
		t.Fatalf("expected cache to remove deleted server, got %+v", service.cfg.Servers)
	}
}

func TestHandleServerByIDGetFallsBackToCachedServerWhenRepositoryReadFails(t *testing.T) {
	service := NewService(&Config{
		Servers: []RemoteServer{{
			ID:       "srv-1",
			Name:     "Cached Production",
			Host:     "13.233.171.43",
			User:     "sai",
			Password: "secret",
		}},
	}, &fakeStore{}, nil)
	service.SetServerRepo(&fakeServerRepository{
		getErr:  &ServerRepositoryError{Op: "get", ID: "srv-1", Err: ErrServerRepositoryNotConfigured},
		servers: map[string]RemoteServer{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/srv-1", nil)
	rec := httptest.NewRecorder()

	service.handleServerByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var srv RemoteServer
	if err := decodeAPIResponseData(rec.Body.Bytes(), &srv); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if srv.ID != "srv-1" {
		t.Fatalf("expected cached server payload, got %+v", srv)
	}
	if srv.Password != "********" {
		t.Fatalf("expected masked password in API payload, got %q", srv.Password)
	}
}

func TestHandleServersPostReturnsConflictWhenRepositoryAlreadyHasServer(t *testing.T) {
	repo := newFakeServerRepository(RemoteServer{
		ID:   "srv-1",
		Name: "Production",
		Host: "13.233.171.43",
		User: "sai",
	})
	service := NewService(&Config{}, &fakeStore{}, nil)
	service.SetServerRepo(repo)

	body, err := json.Marshal(RemoteServer{
		ID:       "srv-1",
		Name:     "Production",
		Host:     "13.233.171.43",
		User:     "sai",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleServers(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleServersPostDoesNotMutateCacheWhenRepositoryIsUnavailable(t *testing.T) {
	repo := newFakeServerRepository()
	repo.createErr = &ServerRepositoryError{Op: "create", ID: "srv-2", Err: ErrServerRepoOffline, Cause: errors.New("mongo unavailable")}

	service := NewService(&Config{
		Servers: []RemoteServer{{
			ID:   "cached-1",
			Name: "Cached Server",
			Host: "10.0.0.2",
			User: "cached",
		}},
	}, &fakeStore{}, nil)
	service.SetServerRepo(repo)

	body := mustMarshalJSON(t, RemoteServer{
		ID:          "srv-2",
		Name:        "Validation",
		Host:        "13.127.106.100",
		User:        "sai",
		PasswordEnv: "SSH_PASSWORD_VALIDATION",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleServers(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.createCalls != 0 {
		t.Fatalf("expected repository create not to report success, got %d calls", repo.createCalls)
	}
	if len(service.cfg.Servers) != 1 || service.cfg.Servers[0].ID != "cached-1" {
		t.Fatalf("expected cache to remain unchanged, got %+v", service.cfg.Servers)
	}
}
