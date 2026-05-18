package googletasks

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/googletasks/googletaskstest"
)

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
		setupFake      func(fake *googletaskstest.FakeGoogleTasks)
		wantErr        bool
		wantExternalID string
	}{
		{
			name: "success",
			list: &model.List{
				Name:     "  New List  \n",
				Modified: time.Now(),
			},
			setupFake:      func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "invalid list (validation failed)",
			list: &model.List{
				Name: "",
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			list: &model.List{
				Name:     "Fail List",
				Modified: time.Now(),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailInsertTaskList = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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
		setupFake func(fake *googletaskstest.FakeGoogleTasks)
		wantLists []model.List
		wantErr   bool
	}{
		{
			name: "success with items",
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
					Id:      "L1",
					Title:   "Inbox",
					Updated: "2024-01-01T12:00:00Z",
				}

				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailListTaskLists = true
			},
			wantLists: nil,
			wantErr:   true,
		},
		{
			name: "list items failure",
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
					Id:      "L1",
					Title:   "Inbox",
					Updated: "2024-01-01T12:00:00Z",
				}

				fake.FailListTasks = true
			},
			wantLists: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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
		setupFake    func(fake *googletaskstest.FakeGoogleTasks)
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Same List",
				}

				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "My List",
				}

				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Same List",
				}

				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{
					Id:    "L1",
					Title: "Source List",
				}

				fake.TaskLists["L2"] = &tasks.TaskList{
					Id:    "L2",
					Title: "Target List",
				}

				fake.Tasks["L1"] = []*tasks.Task{
					{Id: "A"},
				}

				fake.Tasks["L2"] = []*tasks.Task{}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{Id: "L1", Title: "Source List"}
				fake.TaskLists["L2"] = &tasks.TaskList{Id: "L2", Title: "Target List"}
				fake.Tasks["L1"] = []*tasks.Task{{Id: "A"}}
				fake.Tasks["L2"] = []*tasks.Task{{Id: "B"}}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "invalid list (validation failed)",
			list: &model.List{
				ExternalID: stringPtr("L1"),
				Name:       "",
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailPatchTaskList = true
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailMoveTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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

			gotTaskList := fakeTasks.TaskLists[*tt.list.ExternalID]

			if diff := cmp.Diff(tt.wantTaskList, gotTaskList); diff != "" {
				t.Errorf("UpdateList() taskList mismatch (-want +got):\n%s", diff)
			}

			gotTasks := fakeTasks.Tasks[*tt.list.ExternalID]

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
		setupFake func(fake *googletaskstest.FakeGoogleTasks)
		wantErr   bool
	}{
		{
			name: "success",
			list: &model.List{
				ExternalID: stringPtr("L1"),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.TaskLists["L1"] = &tasks.TaskList{Id: "L1"}
			},
			wantErr: false,
		},
		{
			name: "missing external id",
			list: &model.List{
				Name: "Delete List",
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			list: &model.List{
				ExternalID: stringPtr("L1"),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailDeleteTaskList = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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
		setupFake      func(fake *googletaskstest.FakeGoogleTasks)
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
			setupFake:      func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:        false,
			wantExternalID: "generated-task-1",
		},
		{
			name:   "invalid item (validation failed)",
			listID: "L1",
			item: &model.Item{
				Title: "",
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake:      func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake:      func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake:      func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailInsertTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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
		setupFake func(fake *googletaskstest.FakeGoogleTasks)
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t2",
						Title:    "Task 2",
						Position: "0002",
						Status:   "needsAction",
					},
					{
						Id:       "t1",
						Title:    "Task 1",
						Position: "0001",
						Status:   "needsAction",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Alice - Send Mail",
						Position: "0001",
					},
				}
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
			name: "waiting for parsing with created date",
			list: &model.List{
				ID:         "1",
				Name:       "Waiting For",
				ExternalID: stringPtr("L1"),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Alice - Send Mail - Jan 23",
						Position: "0001",
					},
				}
			},
			wantItems: []*model.Item{
				{
					ListID:         "1",
					Title:          "Send Mail",
					WaitingOn:      stringPtr("Alice"),
					Status:         model.StatusNotStarted,
					ExternalID:     stringPtr("t1"),
					ExternalListID: stringPtr("L1"),
					Created: rfc3339ToDate(
						"0000-01-23T00:00:00Z",
					),
				},
			},
		},
		{
			name: "waiting for parsing with invalid created date",
			list: &model.List{
				ID:         "1",
				Name:       "Waiting For",
				ExternalID: stringPtr("L1"),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Alice - Send Mail - Jan 42",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task +ProjectA",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task due:2024-01-01",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task #tag1 #tag2",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task",
						Status:   "completed",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task",
						Notes:    "My notes",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task",
						Due:      "2024-01-01T00:00:00Z",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
					{
						Id:       "t1",
						Title:    "Task",
						Updated:  "2024-01-01T12:00:00Z",
						Position: "0001",
					},
				}
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailListTasks = true
			},
			wantItems: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
			tt.setupFake(fakeTasks)
			mockClient := &http.Client{
				Transport: fakeTasks,
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
		setupFake func(fake *googletaskstest.FakeGoogleTasks)
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "missing external identifiers",
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Update Task",
				Modified: time.Now(),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
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
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailPatchTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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

			idx := slices.IndexFunc(fakeTasks.Tasks[tt.listID], func(t *tasks.Task) bool {
				return t.Id == *tt.item.ExternalID
			})

			if idx == -1 {
				t.Fatalf("expected to find task %s in fake server, but it was missing", *tt.item.ExternalID)
			}

			gotTask := fakeTasks.Tasks[tt.listID][idx]

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
		setupFake func(fake *googletaskstest.FakeGoogleTasks)
		wantErr   bool
	}{
		{
			name: "success",
			item: &model.Item{
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.Tasks["L1"] = []*tasks.Task{{Id: "T1"}}
			},
			wantErr: false,
		},
		{
			name: "missing external identifiers",
			item: &model.Item{
				ListID: "list-1",
				Title:  "Delete Task",
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {},
			wantErr:   true,
		},
		{
			name: "api error",
			item: &model.Item{
				ExternalID:     stringPtr("T1"),
				ExternalListID: stringPtr("L1"),
			},
			setupFake: func(fake *googletaskstest.FakeGoogleTasks) {
				fake.FailDeleteTask = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeTasks := googletaskstest.NewFakeGoogleTasks(t)
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
