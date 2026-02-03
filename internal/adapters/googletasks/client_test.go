package googletasks

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"
)

// mockTransport implements http.RoundTripper to mock API responses
type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestCreateList(t *testing.T) {
	tests := []struct {
		name    string
		list    model.List
		handler func(req *http.Request) *http.Response
		wantErr bool
		wantID  string
	}{
		{
			name: "success",
			list: model.List{
				Name: "New List",
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != "POST" {
					resp := &http.Response{
						StatusCode: 405,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/users/@me/lists" {
					resp := &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
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
			wantErr: false,
			wantID:  "new-list-id",
		},
		{
			name: "api error",
			list: model.List{
				Name: "Fail List",
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			gotID, err := tasksClient.CreateList(context.Background(), tt.list)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && gotID != tt.wantID {
				t.Errorf("CreateList() gotID = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestListLists(t *testing.T) {
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
						StatusCode: 200,
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
						StatusCode: 200,
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
					StatusCode: 404,
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
					Items: []model.Item{
						{
							Title:          "Task 1",
							ExternalID:     stringPtr("T1"),
							Position:       0,
							ListID:         "",
							Status:         model.StatusOpen,
							ExternalListID: stringPtr("L1"),
						},
					},
				},
			},
		},
		{
			name: "tasklists list failure",
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
						StatusCode: 200,
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
					StatusCode: 500,
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
	tests := []struct {
		name         string
		list         model.List
		currentItems []model.Item
		handler      func(req *http.Request) *http.Response
		wantErr      bool
	}{
		{
			name: "success (rename only)",
			list: model.List{
				Name:       "Updated List",
				ExternalID: stringPtr("L1"),
			},
			currentItems: nil,
			handler: func(req *http.Request) *http.Response {
				if req.Method != "PATCH" {
					resp := &http.Response{
						StatusCode: 405,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"title":"Updated List"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Title")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`
						{
							"id": "L1", 
							"title": "Updated List"
						}
					`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "success with reordering",
			list: model.List{
				Name:       "My List",
				ExternalID: stringPtr("L1"),
				Items: []model.Item{
					{
						ExternalID: stringPtr("A"), 
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID: stringPtr("B"), 
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID: stringPtr("C"), 
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []model.Item{
				{ExternalID: stringPtr("B")},
				{ExternalID: stringPtr("C")},
				{ExternalID: stringPtr("A")},
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method == "PATCH" && req.URL.Path == "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L1", 
								"title": "My List"
							}
						`)),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.Method == "POST" && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					if req.URL.Query().Get("previous") == "" {
						resp := &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
							Header:     make(http.Header),
						}

						return resp
					}

					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Wrong Previous")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 400,
					Body:       io.NopCloser(bytes.NewBufferString("Unexpected Request: " + req.URL.String())),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "success with relocation (change list)",
			list: model.List{
				Name:       "Target List",
				ExternalID: stringPtr("L2"),
				Items: []model.Item{
					{
						ExternalID: stringPtr("A"), 
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []model.Item{},
			handler: func(req *http.Request) *http.Response {
				if req.Method == "PATCH" && req.URL.Path == "/tasks/v1/users/@me/lists/L2" {
					resp := &http.Response{
						StatusCode: 200, 
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L2", 
								"title": "Target List"
							}
						`)),
					}

					return resp
				}

				if req.Method == "POST" && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					if req.URL.Query().Get("destinationTasklist") == "L2" {
						resp := &http.Response{
							StatusCode: 200, 
							Body: io.NopCloser(bytes.NewBufferString(`{}`)),
						}

						return resp
					}

					resp := &http.Response{
						StatusCode: 400, 
						Body: io.NopCloser(bytes.NewBufferString("Wrong Destination")),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 400, 
					Body: io.NopCloser(bytes.NewBufferString("Unexpected Request")),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "success with relocation and reorder",
			list: model.List{
				Name:       "Target List",
				ExternalID: stringPtr("L2"),
				Items: []model.Item{
					{
						ExternalID: stringPtr("B"), 
						ExternalListID: stringPtr("L2"),
					},
					{
						ExternalID: stringPtr("A"), 
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []model.Item{
				{ExternalID: stringPtr("B")},
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method == "PATCH" && req.URL.Path == "/tasks/v1/users/@me/lists/L2" {
					resp := &http.Response{
						StatusCode: 200, 
						Body: io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L2", 
								"title": "Target List"
							}
						`))}

					return resp
				}

				if req.Method == "POST" && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					q := req.URL.Query()
					if q.Get("destinationTasklist") == "L2" && q.Get("previous") == "B" {
						resp := &http.Response{
							StatusCode: 200, 
							Body: io.NopCloser(bytes.NewBufferString(`{}`)),
						}

						return resp
					}

					resp := &http.Response{
						StatusCode: 400, 
						Body: io.NopCloser(bytes.NewBufferString("Wrong Params")),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 400, 
					Body: io.NopCloser(bytes.NewBufferString("Unexpected Request")),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name: "update failure",
			list: model.List{
				Name:       "Fail List",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: true,
		},
		{
			name: "move failure",
			list: model.List{
				Name:       "My List",
				ExternalID: stringPtr("L1"),
				Items: []model.Item{
					{
						ExternalID: stringPtr("A"), 
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID: stringPtr("B"), 
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []model.Item{
				{ExternalID: stringPtr("B")},
				{ExternalID: stringPtr("A")},
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method == "PATCH" && req.URL.Path == "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(`
							{
								"id": "L1", 
								"title": "My List"
							}
						`)),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.Method == "POST" && req.URL.Path == "/tasks/v1/lists/L1/tasks/A/move" {
					resp := &http.Response{
						StatusCode: 500,
						Body:       io.NopCloser(bytes.NewBufferString(`{"error": "move failed"}`)),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 400, 
					Body: io.NopCloser(bytes.NewBufferString("Unexpected Request")),
				}

				return resp
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	tests := []struct {
		name    string
		listID  string
		handler func(req *http.Request) *http.Response
		wantErr bool
	}{
		{
			name:   "success",
			listID: "L1",
			handler: func(req *http.Request) *http.Response {
				if req.Method != "DELETE" {
					resp := &http.Response{
						StatusCode: 405,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/users/@me/lists/L1" {
					resp := &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewBufferString("")),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name:   "api error",
			listID: "L1",
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.DeleteList(context.Background(), tt.listID)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteList() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateItem(t *testing.T) {
	tests := []struct {
		name           string
		listID         string
		item           model.Item
		previousItemID string
		handler        func(req *http.Request) *http.Response
		wantErr        bool
		wantID         string
	}{
		{
			name:   "simple item",
			listID: "L1",
			item: model.Item{
				Title:          "Simple",
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != "POST" {
					resp := &http.Response{
						StatusCode: 405,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/lists/L1/tasks" {
					resp := &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"title":"Simple"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Title")),
						Header:     make(http.Header),
					}

					return resp
				}

				if !bytes.Contains(body, []byte(`"status":"needsAction"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Status")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
			wantID:  "T1",
		},
		{
			name:   "completed item",
			listID: "L1",
			item: model.Item{
				Title:          "Done",
				Status:         model.StatusDone,
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"status":"completed"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Status")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
			wantID:  "T1",
		},
		{
			name:   "snoozed item",
			listID: "L1",
			item: model.Item{
				Title:          "Snoozed",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"due":"2024-01-01T00:00:00Z"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Due Date")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
			wantID:  "T1",
		},
		{
			name:   "item with previous",
			listID: "L1",
			item: model.Item{
				Title:          "Task",
				ExternalListID: stringPtr("L1"),
			},
			previousItemID: "P1",
			handler: func(req *http.Request) *http.Response {
				if req.URL.Query().Get("previous") != "P1" {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Previous")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id": "T1"}`)),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
			wantID:  "T1",
		},
		{
			name:   "api error",
			listID: "L1",
			item: model.Item{
				Title:          "Fail",
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			gotID, err := tasksClient.CreateItem(context.Background(), tt.item, tt.previousItemID)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateItem() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && gotID != tt.wantID {
				t.Errorf("CreateItem() gotID = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestListItems(t *testing.T) {
	tests := []struct {
		name      string
		list      model.List
		handler   func(req *http.Request) *http.Response
		wantItems []model.Item
		wantErr   bool
	}{
		{
			name: "basic properties (unsorted, position sort)",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task 1",
					Position:       0,
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
				{
					ListID:         "1",
					Title:          "Task 2",
					Position:       1,
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t2"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "waiting for parsing",
			list: model.List{
				ID:         "1",
				Name:       "Waiting For",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Send Mail",
					WaitingOn:      stringPtr("Alice"),
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "project parsing",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					ProjectID:      stringPtr("ProjectA"),
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "due date parsing (title)",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Due:            iso8601ToDate("2024-01-01"),
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1")},
			},
		},
		{
			name: "multiple tags",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Tags:           []string{"tag1", "tag2"},
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "completed task",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
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
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Description:    "My notes",
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "native due date (snoozed)",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Snoozed:        iso8601ToDate("2024-01-01"),
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "updated timestamp",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 200,
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
			wantItems: []model.Item{
				{
					ListID:         "1",
					Title:          "Task",
					Modified:       rfc3339ToDate("2024-01-01T12:00:00Z"),
					Status:         model.StatusOpen,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
				},
			},
		},
		{
			name: "api error",
			list: model.List{
				ID:         "1",
				Name:       "Inbox",
				ExternalID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
	tests := []struct {
		name    string
		listID  string
		item    model.Item
		handler func(req *http.Request) *http.Response
		wantErr bool
	}{
		{
			name:   "simple item",
			listID: "L1",
			item: model.Item{
				Title:          "Updated Task",
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				if req.Method != "PATCH" {
					resp := &http.Response{
						StatusCode: 405,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/lists/L1/tasks/T1" {
					resp := &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"title":"Updated Task"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Title")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
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
			item: model.Item{
				Title:          "Task",
				Status:         model.StatusDone,
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"status":"completed"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Status")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
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
			item: model.Item{
				Title:          "Task",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				body, _ := io.ReadAll(req.Body)
				if !bytes.Contains(body, []byte(`"due":"2024-01-01T00:00:00Z"`)) {
					resp := &http.Response{
						StatusCode: 400,
						Body:       io.NopCloser(bytes.NewBufferString("Bad Due Date")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 200,
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
			name:   "api error",
			listID: "L1",
			item: model.Item{
				Title:          "Fail",
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
	tests := []struct {
		name    string
		listID  string
		itemID  string
		handler func(req *http.Request) *http.Response
		wantErr bool
	}{
		{
			name:   "success",
			listID: "L1",
			itemID: "T1",
			handler: func(req *http.Request) *http.Response {
				if req.Method != "DELETE" {
					resp := &http.Response{
						StatusCode: 405,
						Body:       io.NopCloser(bytes.NewBufferString("Method Not Allowed")),
						Header:     make(http.Header),
					}

					return resp
				}

				if req.URL.Path != "/tasks/v1/lists/L1/tasks/T1" {
					resp := &http.Response{
						StatusCode: 404,
						Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
						Header:     make(http.Header),
					}

					return resp
				}

				resp := &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewBufferString("")),
					Header:     make(http.Header),
				}

				return resp
			},
			wantErr: false,
		},
		{
			name:   "api error",
			listID: "L1",
			itemID: "T1",
			handler: func(req *http.Request) *http.Response {
				resp := &http.Response{
					StatusCode: 500,
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
			mockClient := &http.Client{
				Transport: &mockTransport{
					roundTripFunc: func(req *http.Request) (*http.Response, error) {
						return tt.handler(req), nil
					},
				},
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			tasksClient := NewClient(tasksService)

			err := tasksClient.DeleteItem(context.Background(), tt.listID, tt.itemID)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteItem() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenderTitle(t *testing.T) {
	tests := []struct {
		name      string
		item      model.Item
		wantTitle string
	}{
		{
			name: "simple title",
			item: model.Item{
				Title: "Simple",
			},
			wantTitle: "Simple",
		},
		{
			name: "title with projectid",
			item: model.Item{
				Title:     "Task",
				ProjectID: stringPtr("P1"),
			},
			wantTitle: "Task +P1",
		},
		{
			name: "title with due",
			item: model.Item{
				Title: "Task",
				Due:   iso8601ToDate("2024-01-01"),
			},
			wantTitle: "Task due:2024-01-01",
		},
		{
			name: "title with multiple tags",
			item: model.Item{
				Title: "Task",
				Tags:  []string{"t1", "t2"},
			},
			wantTitle: "Task #t1 #t2",
		},
		{
			name: "title with waiting on",
			item: model.Item{
				Title:     "Task",
				WaitingOn: stringPtr("Alice"),
				Created:   rfc3339ToDate("2024-01-02T10:00:00Z"),
			},
			wantTitle: "Alice - Task - Jan 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
