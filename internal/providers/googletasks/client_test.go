package googletasks

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// mockTransport implements [http.RoundTripper] to mock API responses.
type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestGetKey(t *testing.T) {
	t.Parallel()
	client := &Client{}

	tests := []struct {
		name     string
		resource model.Resource
		wantKey  string
	}{
		{
			name: "list pointer with id",
			resource: &model.List{
				ExternalID: stringPtr("L1"),
			},
			wantKey: "L1",
		},
		{
			name:     "item with nil external id",
			resource: &model.Item{},
			wantKey:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := client.GetKey(tt.resource)
			if got != tt.wantKey {
				t.Errorf("GetKey() = %v, want %v", got, tt.wantKey)
			}
		})
	}
}

func TestCreateList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		list           *model.List
		handler        func(req *http.Request) *http.Response
		wantErr        bool
		wantExternalID string
	}{
		{
			name: "success",
			list: &model.List{
				Name:     "  New List  \n",
				Modified: time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != http.MethodPost {
					resp := &http.Response{
						StatusCode: http.StatusMethodNotAllowed,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/users/@me/lists" {
					resp := &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"id": "new-list-id",
							"title": "New List"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr:        false,
			wantExternalID: "new-list-id",
		},
		{
			name: "invalid status for new list",
			list: &model.List{
				Name:     "New List",
				Status:   model.StatusDeleted,
				Modified: time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "invalid list (validation failed)",
			list: &model.List{
				Name: "",
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "api error",
			list: &model.List{
				Name:     "Fail List",
				Modified: time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"error": "internal"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.CreateList(context.Background(), tt.list)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if tt.list.ExternalID == nil {
				t.Errorf("CreateList() failed to mutate ExternalID pointer")
			} else if *tt.list.ExternalID != tt.wantExternalID {
				t.Errorf("CreateList() mutated ExternalID = %v, want %v", *tt.list.ExternalID, tt.wantExternalID)
			}
		})
	}
}

func TestListLists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		handler   func(req *http.Request) *http.Response
		wantLists []model.List
		wantErr   bool
	}{
		{
			name: "success with items",
			handler: func(req *http.Request) *http.Response {
				if req.URL.Path == "/tasks/v1/users/@me/lists" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"items": [
									{
										"id": "L1",
										"title": "Inbox",
										"updated": "2024-01-01T12:00:00Z"
									}
								]
							}
						`)),
						Header: make(http.Header),
					}

					return resp
				}

				if req.URL.Path == "/tasks/v1/lists/L1/tasks" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"items": [
									{
										"id": "T1",
										"title": "Task 1",
										"position": "0001"
									}
								]
							}
						`)),
						Header: make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
					Header:     make(http.Header),
				}

				return resp
			},
			wantLists: []model.List{
				{
					Name:       "Inbox",
					ExternalID: stringPtr("L1"),
					Modified:   rfc3339ToDate("2024-01-01T12:00:00Z"),
					Status:     model.StatusOpen,
					Items: []*model.Item{
						{
							Title:          "Task 1",
							ExternalID:     stringPtr("T1"),
							Position:       0,
							ListID:         "",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("L1"),
						},
					},
				},
			},
		},
		{
			name: "tasklists list failure",
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"error": "internal"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantLists: nil,
			wantErr:   true,
		},
		{
			name: "list items failure",
			handler: func(req *http.Request) *http.Response {
				if req.URL.Path == "/tasks/v1/users/@me/lists" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"items": [
									{
										"id":"L1",
										"title":"Inbox"
									}
								]
							}
						`)),
						Header: make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"error": "internal"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantLists: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			got, err := tasksClient.ListLists(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Error("ListLists() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("ListLists() unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tt.wantLists, got); diff != "" {
				t.Errorf("ListLists() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUpdateList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		list         *model.List
		currentItems []*model.Item
		handler      func(req *http.Request) *http.Response
		wantErr      bool
	}{
		{
			name: "success (rename only)",
			list: &model.List{
				Name:       "  Updated List  \n",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			currentItems: nil,
			handler: func(req *http.Request) *http.Response {
				if req.Method != http.MethodPatch {
					resp := &http.Response{
						StatusCode: http.StatusMethodNotAllowed,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"title":"Updated List"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Title")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"id": "L1",
							"title": "Updated List"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "success with reordering",
			list: &model.List{
				Name:       "My List",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("B")},
				{ExternalID: stringPtr("C")},
				{ExternalID: stringPtr("A")},
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method == http.MethodPatch && req.URL.Path == "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L1",
								"title": "My List"
							}
						`)),
						Header: make(http.Header),
					}

					return resp
				}

				if req.Method == http.MethodPost && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					if req.URL.Query().Get("previous") == "" {
						resp := &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
							Header:     make(http.Header),
						}

						return resp
					}

					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Wrong Previous")),
						Header:     make(http.Header),
					}

					return resp
				}

				respBody := "Unexpected Request: " + req.URL.String()
				resp := &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(respBody)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "success with relocation (change list)",
			list: &model.List{
				Name:       "Target List",
				ExternalID: stringPtr("L2"),
				Modified:   time.Now(),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{},
			handler: func(req *http.Request) *http.Response {
				if req.Method == http.MethodPatch && req.URL.Path == "/tasks/v1/users/@me/lists/L2" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L2",
								"title": "Target List"
							}
						`)),
					}

					return resp
				}

				if req.Method == http.MethodPost && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					if req.URL.Query().Get("destinationTasklist") == "L2" {
						resp := &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
						}

						return resp
					}

					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Wrong Destination")),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString("Unexpected Request")),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "success with relocation and reorder",
			list: &model.List{
				Name:       "Target List",
				ExternalID: stringPtr("L2"),
				Modified:   time.Now(),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L2"),
					},
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("B")},
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method == http.MethodPatch && req.URL.Path == "/tasks/v1/users/@me/lists/L2" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L2",
								"title": "Target List"
							}
						`)),
					}

					return resp
				}

				if req.Method == http.MethodPost && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					q := req.URL.Query()
					if q.Get("destinationTasklist") == "L2" && q.Get("previous") == "B" {
						resp := &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
						}

						return resp
					}

					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Wrong Params")),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString("Unexpected Request")),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "missing external id",
			list: &model.List{
				Name:     "Update List",
				Modified: time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "invalid list (validation failed)",
			list: &model.List{
				ExternalID: stringPtr("L1"),
				Name:       "",
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "update failure",
			list: &model.List{
				Name:       "Fail List",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
		{
			name: "move failure",
			list: &model.List{
				Name:       "My List",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("B")},
				{ExternalID: stringPtr("A")},
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method == http.MethodPatch && req.URL.Path == "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L1",
								"title": "My List"
							}
						`)),
						Header: make(http.Header),
					}

					return resp
				}

				if req.Method == http.MethodPost && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					resp := &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(bytes.NewBufferString(`{"error": "move failed"}`)),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString("Unexpected Request")),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.UpdateList(context.Background(), tt.list, tt.currentItems)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateList() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		list    *model.List
		handler func(req *http.Request) *http.Response
		wantErr bool
	}{
		{
			name: "success",
			list: &model.List{
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != http.MethodDelete {
					resp := &http.Response{
						StatusCode: http.StatusMethodNotAllowed,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(bytes.NewBufferString("")),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "missing external id",
			list: &model.List{
				Name: "Delete List",
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "api error",
			list: &model.List{
				ExternalID: stringPtr("L1"),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.DeleteList(context.Background(), tt.list)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteList() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		listID         string
		item           *model.Item
		previousItemID string
		handler        func(req *http.Request) *http.Response
		wantErr        bool
		wantExternalID string
	}{
		{
			name:   "simple item",
			listID: "L1",
			item: &model.Item{
				Title:          "  Simple  \n",
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != http.MethodPost {
					resp := &http.Response{
						StatusCode: http.StatusMethodNotAllowed,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/lists/L1/tasks" {
					resp := &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"title":"Simple"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Title")),
						Header:     make(http.Header),
					}

					return resp
				}

				if !bytes.Contains(body, []byte(`"status":"needsAction"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Status")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr:        false,
			wantExternalID: "T1",
		},
		{
			name:   "invalid item (validation failed)",
			listID: "L1",
			item: &model.Item{
				Title: "",
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name:   "completed item",
			listID: "L1",
			item: &model.Item{
				Title:          "Done",
				Status:         model.StatusDone,
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"status":"completed"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Status")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr:        false,
			wantExternalID: "T1",
		},
		{
			name:   "snoozed item",
			listID: "L1",
			item: &model.Item{
				Title:          "Snoozed",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"due":"2024-01-01T00:00:00Z"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Due Date")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr:        false,
			wantExternalID: "T1",
		},
		{
			name:   "item with previous",
			listID: "L1",
			item: &model.Item{
				Title:          "Task",
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			previousItemID: "P1",
			handler: func(req *http.Request) *http.Response {
				if req.URL.Query().Get("previous") != "P1" {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Previous")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr:        false,
			wantExternalID: "T1",
		},
		{
			name:   "cannot create deleted item",
			listID: "L1",
			item: &model.Item{
				ListID:         "list-1",
				Title:          "Deleted Task",
				Status:         model.StatusDeleted,
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name:   "missing external list id",
			listID: "L1",
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Fail",
				Modified: time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name:   "api error",
			listID: "L1",
			item: &model.Item{
				ListID:         "list-1",
				Title:          "Fail",
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.CreateItem(context.Background(), tt.item, tt.previousItemID)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateItem() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if tt.item.ExternalID == nil {
				t.Errorf("CreateItem() failed to mutate ExternalID pointer")
			} else if *tt.item.ExternalID != tt.wantExternalID {
				t.Errorf("CreateItem() mutated ExternalID = %v, want %v", *tt.item.ExternalID, tt.wantExternalID)
			}
		})
	}
}

func TestListItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		list      *model.List
		handler   func(req *http.Request) *http.Response
		wantItems []*model.Item
		wantErr   bool
	}{
		{
			name: "basic properties (unsorted, position sort)",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t2",
									"title": "Task 2",
									"position": "0002",
									"status": "needsAction"
								},
								{
									"id": "t1",
									"title": "Task 1",
									"position": "0001",
									"status": "needsAction"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task 1",
					Position:       0,
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
				{
					ListID:         "1",
					Title:          "Task 2",
					Position:       1,
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t2"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "waiting for parsing",
			list: &model.List{
				ID:         "1",
				Name:       "Waiting For",
				ExternalID: stringPtr("L1"),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Alice - Send Mail - Jan 23",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Send Mail",
					WaitingOn:      stringPtr("Alice"),
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "project parsing",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task +ProjectA",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					ProjectID:      stringPtr("ProjectA"),
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "due date parsing (title)",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task due:2024-01-01",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Due:            iso8601ToDate("2024-01-01"),
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "multiple tags",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task #tag1 #tag2",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Tags:           []string{"tag1", "tag2"},
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "completed task",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task",
									"status": "completed",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Status:         model.StatusDone,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "description included",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task",
									"notes": "My notes",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Description:    "My notes",
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "native due date (snoozed)",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task",
									"due": "2024-01-01T00:00:00Z",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Snoozed:        iso8601ToDate("2024-01-01"),
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "updated timestamp",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"items": [
								{
									"id": "t1",
									"title": "Task",
									"updated": "2024-01-01T12:00:00Z",
									"position": "0001"
								}
							]
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Modified:       rfc3339ToDate("2024-01-01T12:00:00Z"),
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "api error",
			list: &model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"error": "internal"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantItems: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksSerice, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksSerice)

			got, err := tasksClient.listItems(context.Background(), tt.list)

			if tt.wantErr {
				if err == nil {
					t.Error("listItems() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("listItems() unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tt.wantItems, got); diff != "" {
				t.Errorf("listItems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUpdateItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		listID  string
		item    *model.Item
		handler func(req *http.Request) *http.Response
		wantErr bool
	}{
		{
			name:   "simple item",
			listID: "L1",
			item: &model.Item{
				Title:          "  Updated Task  \n",
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != http.MethodPatch {
					resp := &http.Response{
						StatusCode: http.StatusMethodNotAllowed,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/lists/L1/tasks/T1" {
					resp := &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"title":"Updated Task"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Title")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"id": "T1",
							"title": "Updated Task"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name:   "completed item",
			listID: "L1",
			item: &model.Item{
				Title:          "Task",
				Status:         model.StatusDone,
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"status":"completed"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Status")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"id": "T1",
							"status": "completed"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name:   "snoozed item",
			listID: "L1",
			item: &model.Item{
				Title:          "Task",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"due":"2024-01-01T00:00:00Z"`)) {
					resp := &http.Response{
						StatusCode: http.StatusBadRequest,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Due Date")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"id": "T1"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "invalid item (validation failed)",
			item: &model.Item{
				Title: "",
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "missing external identifiers",
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Update Task",
				Modified: time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "api error",
			item: &model.Item{
				ListID:         "L1",
				Title:          "Fail",
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body: io.NopCloser(bytes.NewBufferString(`
						{
							"error": "internal"
						}
					`)),
					Header: make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.UpdateItem(context.Background(), tt.item)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateItem() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		item    *model.Item
		handler func(req *http.Request) *http.Response
		wantErr bool
	}{
		{
			name: "success",
			item: &model.Item{
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != http.MethodDelete {
					resp := &http.Response{
						StatusCode: http.StatusMethodNotAllowed,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/lists/L1/tasks/T1" {
					resp := &http.Response{
						StatusCode: http.StatusNotFound,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(bytes.NewBufferString("")),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "missing external identifiers",
			item: &model.Item{
				ListID: "list-1",
				Title:  "Delete Task",
			},
			handler: func(_ *http.Request) *http.Response {
				return nil
			},
			wantErr: true,
		},
		{
			name: "api error",
			item: &model.Item{
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(_ *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.DeleteItem(context.Background(), tt.item)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteItem() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenderTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		item      *model.Item
		wantTitle string
	}{
		{
			name: "simple title",
			item: &model.Item{
				Title: "Simple",
			},
			wantTitle: "Simple",
		},
		{
			name: "title with projectid",
			item: &model.Item{
				Title:     "Task",
				ProjectID: stringPtr("P1"),
			},
			wantTitle: "Task +P1",
		},
		{
			name: "title with due",
			item: &model.Item{
				Title: "Task",
				Due:   iso8601ToDate("2024-12-31"),
			},
			wantTitle: "Task due:2024-12-31",
		},
		{
			name: "title with multiple tags",
			item: &model.Item{
				Title: "Task",
				Tags:  []string{"t1", "t2"},
			},
			wantTitle: "Task #t1 #t2",
		},
		{
			name: "title with waiting on",
			item: &model.Item{
				Title:     "Task",
				WaitingOn: stringPtr("Alice"),
				Created:   rfc3339ToDate("2024-01-02T10:00:00Z"),
			},
			wantTitle: "Alice - Task - Jan 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := renderTitle(tt.item)
			if got != tt.wantTitle {
				t.Errorf("renderTitle() = %q, want %q", got, tt.wantTitle)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func iso8601ToDate(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)

	return &t
}

func rfc3339ToDate(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)

	return t
}
