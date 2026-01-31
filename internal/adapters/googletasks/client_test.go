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

			err := tasksClient.CreateList(context.Background(), tt.list)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateList() error = %v, wantErr %v", err, tt.wantErr)
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
					Modified:   rfc3339("2024-01-01T12:00:00Z"),
					Items: []model.Item{
						{
							Title:      "Task 1",
							ExternalID: stringPtr("T1"),
							Position:   0,
							ListID:     0,
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
				ID:         1,
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
					ListID:     1,
					Title:      "Task 1",
					Position:   0,
					ExternalID: stringPtr("t1"),
				},
				{
					ListID:     1,
					Title:      "Task 2",
					Position:   1,
					ExternalID: stringPtr("t2"),
				},
			},
		},
		{
			name: "waiting for parsing",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Send Mail",
					WaitingOn:  stringPtr("Alice"),
					ExternalID: stringPtr("t1"),
				},
			},
		},
		{
			name: "project parsing",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Task",
					ProjectID:  stringPtr("ProjectA"),
					ExternalID: stringPtr("t1"),
				},
			},
		},
		{
			name: "due date parsing (title)",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Task",
					Due:        date("2024-01-01"),
					ExternalID: stringPtr("t1")},
			},
		},
		{
			name: "multiple tags",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Task",
					Tags:       []string{"tag1", "tag2"},
					ExternalID: stringPtr("t1"),
				},
			},
		},
		{
			name: "completed task",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Task",
					Completed:  true,
					ExternalID: stringPtr("t1"),
				},
			},
		},
		{
			name: "description included",
			list: model.List{
				ID:         1,
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
					ListID:      1,
					Title:       "Task",
					Description: "My notes",
					ExternalID:  stringPtr("t1"),
				},
			},
		},
		{
			name: "native due date (snoozed)",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Task",
					Snoozed:    date("2024-01-01"),
					ExternalID: stringPtr("t1"),
				},
			},
		},
		{
			name: "updated timestamp",
			list: model.List{
				ID:         1,
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
					ListID:     1,
					Title:      "Task",
					Modified:   rfc3339("2024-01-01T12:00:00Z"),
					ExternalID: stringPtr("t1"),
				},
			},
		},
		{
			name: "api error",
			list: model.List{
				ID:         1,
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

			got, err := tasksClient.ListItems(context.Background(), tt.list)

			if tt.wantErr {
				if err == nil {
					t.Error("ListItems() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("ListItems() unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tt.wantItems, got); diff != "" {
				t.Errorf("ListItems() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func date(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)

	return &t
}

func rfc3339(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)

	return t
}
