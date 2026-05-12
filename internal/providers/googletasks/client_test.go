package googletasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/model"
)

var (
	taskListRegex = regexp.MustCompile(`^/tasks/v1/users/@me/lists(?:/([^/]+))?$`)
	taskRegex     = regexp.MustCompile(`^/tasks/v1/lists/([^/]+)/tasks(?:/([^/]+)(?:/move)?)?$`)
)

// mockTransport implements [http.RoundTripper] to mock API responses.
type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

type FakeGoogleTasks struct {
	t                  *testing.T
	taskLists          map[string]*tasks.TaskList
	tasks              map[string][]*tasks.Task
	failInsertTaskList bool
	failListTaskLists  bool
	failPatchTaskList  bool
	failDeleteTaskList bool
	failInsertTask     bool
	failListTasks      bool
	failPatchTask      bool
	failMoveTask       bool
	failDeleteTask     bool
}

func NewFakeGoogleTasks(t *testing.T) *FakeGoogleTasks {
	googleTasks := &FakeGoogleTasks{
		t:         t,
		taskLists: map[string]*tasks.TaskList{},
		tasks:     map[string][]*tasks.Task{},
	}

	return googleTasks
}

func (f *FakeGoogleTasks) RoundTrip(req *http.Request) (*http.Response, error) {
	if matches := taskListRegex.FindStringSubmatch(req.URL.Path); matches != nil {
		taskListID := matches[1]

		switch req.Method {
		case http.MethodPost:
			return f.InsertTaskList(req.Body)
		case http.MethodGet:
			return f.ListTaskLists()
		case http.MethodPatch:
			return f.PatchTaskList(taskListID, req.Body)
		case http.MethodDelete:
			return f.DeleteTaskList(taskListID)
		}
	}

	if matches := taskRegex.FindStringSubmatch(req.URL.Path); matches != nil {
		taskListID := matches[1]
		taskID := matches[2]

		switch req.Method {
		case http.MethodPost:
			if taskID == "" {
				return f.InsertTask(taskListID, req.Body)
			}

			return f.MoveTask(taskListID, taskID, req)
		case http.MethodGet:
			return f.ListTasks(taskListID)
		case http.MethodPatch:
			return f.PatchTask(taskListID, taskID, req.Body)
		case http.MethodDelete:
			return f.DeleteTask(taskListID, taskID)
		}
	}

	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewBufferString("Not Found: " + req.URL.Path)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) InsertTaskList(reqBody io.Reader) (*http.Response, error) {
	if f.failInsertTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(reqBody)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	var taskList tasks.TaskList
	if err = json.Unmarshal(body, &taskList); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
	}

	taskList.Id = fmt.Sprintf("generated-list-%d", len(f.taskLists)+1)
	f.taskLists[taskList.Id] = &taskList

	respBody, err := json.Marshal(&taskList)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) ListTaskLists() (*http.Response, error) {
	if f.failListTaskLists {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	taskLists := &tasks.TaskLists{
		Items: slices.Collect(maps.Values(f.taskLists)),
	}

	body, err := json.Marshal(taskLists)
	if err != nil {
		f.t.Fatalf("failed to marshal tasklists: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) PatchTaskList(taskListID string, reqBody io.Reader) (*http.Response, error) {
	if f.failPatchTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(reqBody)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	taskList, ok := f.taskLists[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	if err = json.Unmarshal(body, &taskList); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
	}

	respBody, err := json.Marshal(&taskList)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) DeleteTaskList(taskListID string) (*http.Response, error) {
	if f.failDeleteTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	delete(f.taskLists, taskListID)
	delete(f.tasks, taskListID)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) InsertTask(taskListID string, reqBody io.Reader) (*http.Response, error) {
	if f.failInsertTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(reqBody)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	var task tasks.Task
	if err = json.Unmarshal(body, &task); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
	}

	task.Id = fmt.Sprintf("generated-task-%d", len(f.taskLists)+1)
	f.tasks[taskListID] = append(f.tasks[taskListID], &task)

	respBody, err := json.Marshal(&task)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) ListTasks(taskListID string) (*http.Response, error) {
	if f.failListTasks {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	taskItems := &tasks.Tasks{
		Items: f.tasks[taskListID],
	}

	body, err := json.Marshal(taskItems)
	if err != nil {
		f.t.Fatalf("failed to marshal tasks: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) PatchTask(taskListID, taskID string, reqBody io.Reader) (*http.Response, error) {
	if f.failPatchTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(reqBody)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	idx := slices.IndexFunc(f.tasks[taskListID], func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	if idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	task := f.tasks[taskListID][idx]
	if err = json.Unmarshal(body, &task); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
	}

	if bytes.Contains(body, []byte(`"notes":null`)) {
		task.Notes = ""
	}

	if bytes.Contains(body, []byte(`"due":null`)) {
		task.Due = ""
	}

	respBody, err := json.Marshal(&task)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) MoveTask(taskListID, taskID string, req *http.Request) (*http.Response, error) {
	if f.failMoveTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	q := req.URL.Query()
	destTaskListID := q.Get("destinationTasklist")
	prevTaskID := q.Get("previous")
	if destTaskListID == "" {
		destTaskListID = taskListID
	}

	idx := slices.IndexFunc(f.tasks[taskListID], func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	if idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	task := f.tasks[taskListID][idx]
	f.tasks[taskListID] = slices.Delete(f.tasks[taskListID], idx, idx+1)

	prevTaskIdx := slices.IndexFunc(f.tasks[destTaskListID], func(t *tasks.Task) bool {
		return t.Id == prevTaskID
	})

	f.tasks[destTaskListID] = slices.Insert(f.tasks[destTaskListID], prevTaskIdx+1, task)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) DeleteTask(taskListID, taskID string) (*http.Response, error) {
	if f.failDeleteTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	f.tasks[taskListID] = slices.DeleteFunc(f.tasks[taskListID], func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
	}

	return resp, nil
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
		setupFake      func(fake *FakeGoogleTasks)
		wantErr        bool
		wantExternalID string
	}{
		{
			name: "success",
			list: &model.List{
				Name:     "  New List  \n",
				Modified: time.Now(),
			},
			setupFake:      func(fake *FakeGoogleTasks) {},
			wantErr:        false,
			wantExternalID: "generated-list-1",
		},
		{
			name: "invalid status for new list",
			list: &model.List{
				Name:     "New List",
				Status:   model.StatusDeleted,
				Modified: time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "invalid list (validation failed)",
			list: &model.List{
				Name: "",
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			list: &model.List{
				Name:     "Fail List",
				Modified: time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failInsertTaskList = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

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
		setupFake func(fake *FakeGoogleTasks)
		wantLists []model.List
		wantErr   bool
	}{
		{
			name: "success with items",
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:      "L1",
					Title:   "Inbox",
					Updated: "2024-01-01T12:00:00Z",
				}

				fake.tasks["L1"] = []*tasks.Task{
					{
						Id:       "T1",
						Title:    "Task 1",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failListTaskLists = true
			},
			wantLists: nil,
			wantErr:   true,
		},
		{
			name: "list items failure",
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:      "L1",
					Title:   "Inbox",
					Updated: "2024-01-01T12:00:00Z",
				}

				fake.failListTasks = true
			},
			wantLists: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

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
		currentList  *model.List
		setupFake    func(fake *FakeGoogleTasks)
		wantTaskList *tasks.TaskList
		wantTasks    []*tasks.Task
		wantErr      bool
	}{
		{
			name: "success (no updates needed)",
			list: &model.List{
				Name:       "Same List",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
				Items: []*model.Item{
					{
						Title:          "Task 1",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task 2",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentList: &model.List{
				Name: "Same List",
				Items: []*model.Item{
					{
						Title:          "Task 2",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task 1",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Same List",
				}

				fake.tasks["L1"] = []*tasks.Task{
					{
						Id:     "B",
						Title:  "Task 2",
						Status: "completed",
					},
					{
						Id:     "A",
						Title:  "Task 1",
						Status: "completed",
					},
				}
			},
			wantTaskList: &tasks.TaskList{
				Id:    "L1",
				Title: "Same List",
			},
			wantTasks: []*tasks.Task{
				{
					Id:     "B",
					Title:  "Task 2",
					Status: "completed",
				},
				{
					Id:     "A",
					Title:  "Task 1",
					Status: "completed",
				},
			},
			wantErr: false,
		},
		{
			name: "success (rename only)",
			list: &model.List{
				Name:       "  Updated List  \n",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			currentList: &model.List{
				Name: "Target List",
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Target List",
				}
			},
			wantTaskList: &tasks.TaskList{
				Id:    "L1",
				Title: "Updated List",
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
			currentList: &model.List{
				Name: "My List",
				Items: []*model.Item{
					{ExternalID: stringPtr("B")},
					{ExternalID: stringPtr("C")},
					{ExternalID: stringPtr("A")},
				},
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "My List",
				}

				fake.tasks["L1"] = []*tasks.Task{
					{Id: "B"},
					{Id: "C"},
					{Id: "A"},
				}
			},
			wantTaskList: &tasks.TaskList{
				Id:    "L1",
				Title: "My List",
			},
			wantTasks: []*tasks.Task{
				{Id: "A"},
				{Id: "B"},
				{Id: "C"},
			},
			wantErr: false,
		},
		{
			name: "success (completed items ignored during reorder)",
			list: &model.List{
				Name:       "Same List",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
				Items: []*model.Item{
					{
						Title:          "Task B",
						Status:         model.StatusInProgress,
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task A",
						Status:         model.StatusInProgress,
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task D",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("D"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task C",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentList: &model.List{
				Name: "Same List",
				Items: []*model.Item{
					{
						Title:          "Task A",
						Status:         model.StatusInProgress,
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task B",
						Status:         model.StatusInProgress,
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task C",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
					{
						Title:          "Task D",
						Status:         model.StatusDone,
						ExternalID:     stringPtr("D"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Same List",
				}

				fake.tasks["L1"] = []*tasks.Task{
					{Id: "A"},
					{Id: "B"},
					{Id: "C"},
					{Id: "D"},
				}
			},
			wantTaskList: &tasks.TaskList{
				Id:    "L1",
				Title: "Same List",
			},
			wantTasks: []*tasks.Task{
				{Id: "B"},
				{Id: "A"},
				{Id: "C"},
				{Id: "D"},
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
			currentList: &model.List{
				Name:  "Target List",
				Items: []*model.Item{},
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Source List",
				}

				fake.taskLists["L2"] = &tasks.TaskList{
					Id:    "L2",
					Title: "Target List",
				}

				fake.tasks["L1"] = []*tasks.Task{
					{Id: "A"},
				}

				fake.tasks["L2"] = []*tasks.Task{}
			},
			wantTaskList: &tasks.TaskList{
				Id:    "L2",
				Title: "Target List",
			},
			wantTasks: []*tasks.Task{
				{Id: "A"},
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
			currentList: &model.List{
				Name: "Target List",
				Items: []*model.Item{
					{ExternalID: stringPtr("B")},
				},
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{Id: "L1", Title: "Source List"}
				fake.taskLists["L2"] = &tasks.TaskList{Id: "L2", Title: "Target List"}
				fake.tasks["L1"] = []*tasks.Task{{Id: "A"}}
				fake.tasks["L2"] = []*tasks.Task{{Id: "B"}}
			},
			wantTaskList: &tasks.TaskList{
				Id:    "L2",
				Title: "Target List",
			},
			wantTasks: []*tasks.Task{
				{Id: "B"},
				{Id: "A"},
			},
			wantErr: false,
		},
		{
			name: "missing external id",
			list: &model.List{
				Name:     "Update List",
				Modified: time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "invalid list (validation failed)",
			list: &model.List{
				ExternalID: stringPtr("L1"),
				Name:       "",
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "update failure",
			list: &model.List{
				Name:       "Fail List",
				ExternalID: stringPtr("L1"),
				Modified:   time.Now(),
			},
			currentList: &model.List{
				Name: "Target List",
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failPatchTaskList = true
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
			currentList: &model.List{
				Name: "My List",
				Items: []*model.Item{
					{ExternalID: stringPtr("B")},
					{ExternalID: stringPtr("A")},
				},
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failMoveTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

			err := tasksClient.UpdateList(context.Background(), tt.list, tt.currentList)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			gotTaskList := fakeTasks.taskLists[*tt.list.ExternalID]

			if diff := cmp.Diff(tt.wantTaskList, gotTaskList); diff != "" {
				t.Errorf("UpdateList() taskList mismatch (-want +got):\n%s", diff)
			}

			gotTasks := fakeTasks.tasks[*tt.list.ExternalID]

			if diff := cmp.Diff(tt.wantTasks, gotTasks); diff != "" {
				t.Errorf("UpdateList() tasks mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDeleteList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		list      *model.List
		setupFake func(fake *FakeGoogleTasks)
		wantErr   bool
	}{
		{
			name: "success",
			list: &model.List{
				ExternalID: stringPtr("L1"),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.taskLists["L1"] = &tasks.TaskList{Id: "L1"}
			},
			wantErr: false,
		},
		{
			name: "missing external id",
			list: &model.List{
				Name: "Delete List",
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			list: &model.List{
				ExternalID: stringPtr("L1"),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failDeleteTaskList = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

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
		setupFake      func(fake *FakeGoogleTasks)
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
			setupFake:      func(fake *FakeGoogleTasks) {},
			wantErr:        false,
			wantExternalID: "generated-task-1",
		},
		{
			name:   "invalid item (validation failed)",
			listID: "L1",
			item: &model.Item{
				Title: "",
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
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
			setupFake:      func(fake *FakeGoogleTasks) {},
			wantErr:        false,
			wantExternalID: "generated-task-1",
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
			setupFake:      func(fake *FakeGoogleTasks) {},
			wantErr:        false,
			wantExternalID: "generated-task-1",
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
			setupFake:      func(fake *FakeGoogleTasks) {},
			wantErr:        false,
			wantExternalID: "generated-task-1",
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
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name:   "missing external list id",
			listID: "L1",
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Fail",
				Modified: time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
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
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failInsertTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

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

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

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
		name      string
		listID    string
		item      *model.Item
		setupFake func(fake *FakeGoogleTasks)
		wantTask  *tasks.Task
		wantErr   bool
	}{
		{
			name:   "simple item",
			listID: "L1",
			item: &model.Item{
				Title:          "  Updated Task  \n",
				Description:    "Has desc",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.tasks["L1"] = []*tasks.Task{
					{
						Id:    "T1",
						Title: "Old Title",
					},
				}
			},
			wantTask: &tasks.Task{
				Id:     "T1",
				Title:  "Updated Task",
				Notes:  "Has desc",
				Due:    "2024-01-01T00:00:00Z",
				Status: "needsAction",
			},
			wantErr: false,
		},
		{
			name:   "completed item",
			listID: "L1",
			item: &model.Item{
				Title:          "Task",
				Description:    "Has desc",
				Snoozed:        iso8601ToDate("2024-01-01"),
				Status:         model.StatusDone,
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.tasks["L1"] = []*tasks.Task{
					{
						Id:     "T1",
						Status: "needsAction",
					},
				}
			},
			wantTask: &tasks.Task{
				Id:     "T1",
				Title:  "Task",
				Notes:  "Has desc",
				Due:    "2024-01-01T00:00:00Z",
				Status: "completed",
			},
			wantErr: false,
		},
		{
			name:   "snoozed item",
			listID: "L1",
			item: &model.Item{
				Title:          "Task",
				Description:    "Has desc",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.tasks["L1"] = []*tasks.Task{
					{Id: "T1"},
				}
			},
			wantTask: &tasks.Task{
				Id:     "T1",
				Title:  "Task",
				Notes:  "Has desc",
				Due:    "2024-01-01T00:00:00Z",
				Status: "needsAction",
			},
			wantErr: false,
		},
		{
			name:   "clear description and snoozed date",
			listID: "L1",
			item: &model.Item{
				Title:          "Task",
				Description:    "",
				Snoozed:        nil,
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.tasks["L1"] = []*tasks.Task{
					{
						Id:    "T1",
						Due:   "2024-01-01",
						Notes: "Old note",
					},
				}
			},
			wantTask: &tasks.Task{
				Id:     "T1",
				Title:  "Task",
				Due:    "",
				Notes:  "",
				Status: "needsAction",
			},
			wantErr: false,
		},
		{
			name: "invalid item (validation failed)",
			item: &model.Item{
				Title: "",
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "missing external identifiers",
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Update Task",
				Modified: time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			item: &model.Item{
				ListID:         "L1",
				Title:          "Fail",
				Description:    "Has desc",
				Snoozed:        iso8601ToDate("2024-01-01"),
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
				Modified:       time.Now(),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failPatchTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

			err := tasksClient.UpdateItem(context.Background(), tt.item)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateItem() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			idx := slices.IndexFunc(fakeTasks.tasks[tt.listID], func(t *tasks.Task) bool {
				return t.Id == *tt.item.ExternalID
			})

			if idx == -1 {
				t.Fatalf("expected to find task %s in fake server, but it was missing", *tt.item.ExternalID)
			}

			gotTask := fakeTasks.tasks[tt.listID][idx]

			if diff := cmp.Diff(tt.wantTask, gotTask); diff != "" {
				t.Errorf("Updated item mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDeleteItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		item      *model.Item
		setupFake func(fake *FakeGoogleTasks)
		wantErr   bool
	}{
		{
			name: "success",
			item: &model.Item{
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.tasks["L1"] = []*tasks.Task{{Id: "T1"}}
			},
			wantErr: false,
		},
		{
			name: "missing external identifiers",
			item: &model.Item{
				ListID: "list-1",
				Title:  "Delete Task",
			},
			setupFake: func(fake *FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			item: &model.Item{
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			setupFake: func(fake *FakeGoogleTasks) {
				fake.failDeleteTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
			}

			tasksService, _ := tasks.NewService(context.Background(), option.WithHTTPClient(mockClient))
			pollInterval := 30 * time.Second
			logger := slog.New(slog.DiscardHandler)
			tasksClient := NewClient(tasksService, pollInterval, logger)

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
