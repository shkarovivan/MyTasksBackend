package main

// Sync wire format. JSON tags mirror exactly what the app's Gson produces
// (camelCase deadLine/deadLineMs, enums as "STARTED"/"WORK" name() strings);
// type/status/work values are stored and returned verbatim, casing included.

type SyncTask struct {
	ID          string `json:"id"`
	Created     string `json:"created"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	DeadLine    string `json:"deadLine"`
	DeadLineMs  int64  `json:"deadLineMs"`
	Status      string `json:"status"`
	Work        string `json:"work"`
	UpdatedAt   int64  `json:"updatedAt"`
	Deleted     bool   `json:"deleted"`
}

type SyncPushRequest struct {
	Tasks []SyncTask `json:"tasks"`
}

type SyncPushResponse struct {
	Applied int `json:"applied"`
}

type SyncPullResponse struct {
	Tasks  []SyncTask `json:"tasks"`
	Cursor int64      `json:"cursor"`
}
