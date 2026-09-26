package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncTaskJSONTags(t *testing.T) {
	// The wire names must match the app's Gson output exactly.
	raw := `{"id":"t1","created":"01.01","title":"T","description":"D","type":"daily",
		"deadLine":"01.02","deadLineMs":1000,"status":"STARTED","work":"WORK",
		"updatedAt":123,"deleted":false}`
	var task SyncTask
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if task.DeadLine != "01.02" || task.DeadLineMs != 1000 || task.UpdatedAt != 123 {
		t.Fatalf("camelCase tags broken: %+v", task)
	}
	out, _ := json.Marshal(task)
	for _, want := range []string{`"deadLine"`, `"deadLineMs"`, `"updatedAt"`, `"deleted"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
}

func TestSyncPushValidation(t *testing.T) {
	s := &server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/push", strings.NewReader(`{"tasks":[]}`))
	rec := httptest.NewRecorder()
	s.handleSyncPush(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty tasks: got %d, want 400", rec.Code)
	}

	body := `{"tasks":[{"id":"","updatedAt":100}]}`
	req = httptest.NewRequest(http.MethodPost, "/v1/sync/push", strings.NewReader(body))
	rec = httptest.NewRecorder()
	s.handleSyncPush(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty id: got %d, want 400", rec.Code)
	}

	body = `{"tasks":[{"id":"a","updatedAt":0}]}`
	req = httptest.NewRequest(http.MethodPost, "/v1/sync/push", strings.NewReader(body))
	rec = httptest.NewRecorder()
	s.handleSyncPush(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zero updatedAt: got %d, want 400", rec.Code)
	}
}

func TestSyncPullParamValidation(t *testing.T) {
	s := &server{}
	for _, q := range []string{"since=-1", "since=abc", "limit=0", "limit=501", "limit=xyz"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/sync/pull?"+q, nil)
		rec := httptest.NewRecorder()
		s.handleSyncPull(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want 400", q, rec.Code)
		}
	}
}
