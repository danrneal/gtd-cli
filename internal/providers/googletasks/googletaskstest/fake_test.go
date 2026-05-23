package googletaskstest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/tasks/v1"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(fake *FakeGoogleTasks)
		req           *http.Request
		wantStatus    int
		wantTaskLists []*tasks.TaskList
		wantTasks     map[string][]*tasks.Task
	}{
		{
			name: "insert task list",
			req: httptest.NewRequest(http.MethodPost, "/tasks/v1/users/@me/lists", strings.NewReader(`
				{
					"title": "My List"
				}
			`)),
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "My List",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {},
			},
		},
		{
			name: "fails to insert task list",
			setup: func(fake *FakeGoogleTasks) {
				fake.FailInsertTaskList = true
			},
			req: httptest.NewRequest(http.MethodPost, "/tasks/v1/users/@me/lists", strings.NewReader(`
				{
					"title": "My List"
				}
			`)),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "list task lists",
			req:  httptest.NewRequest(http.MethodGet, "/tasks/v1/users/@me/lists", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
		},
		{
			name: "fails to list task lists",
			req:  httptest.NewRequest(http.MethodGet, "/tasks/v1/users/@me/lists", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailListTaskLists = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "patch task list",
			req: httptest.NewRequest(http.MethodPatch, "/tasks/v1/users/@me/lists/external-list-1", strings.NewReader(`
				{
					"title": "My Updated List"
				}
			`)),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "My Updated List",
				},
			},
		},
		{
			name: "patch task list not found",
			req: httptest.NewRequest(http.MethodPatch, "/tasks/v1/users/@me/lists/external-list-2", strings.NewReader(`
				{
					"title": "My Updated List"
				}
			`)),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
		},
		{
			name: "fails to patch task list",
			req: httptest.NewRequest(http.MethodPatch, "/tasks/v1/users/@me/lists/external-list-1", strings.NewReader(`
				{
					"title": "My Updated List"
				}
			`)),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailPatchTaskList = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "delete task list",
			req:  httptest.NewRequest(http.MethodDelete, "/tasks/v1/users/@me/lists/external-list-1", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "delete task list not found",
			req:  httptest.NewRequest(http.MethodDelete, "/tasks/v1/users/@me/lists/external-list-2", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
		},
		{
			name: "fails to delete task list",
			req:  httptest.NewRequest(http.MethodDelete, "/tasks/v1/users/@me/lists/external-list-1", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailDeleteTaskList = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "insert task",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks",
				strings.NewReader(`
					{
						"title": "T1"
					}
				`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {},
				}
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "insert task list not found",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-2/tasks",
				strings.NewReader(`
					{
						"title": "T1"
					}
				`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {},
			},
		},
		{
			name: "insert task with previous",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks?previous=external-task-1",
				strings.NewReader(`
					{
						"title": "T2"
					}
				`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}

				fake.taskCounter++
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
					{
						Id:    "external-task-2",
						Title: "T2",
					},
				},
			},
		},
		{
			name: "insert task previous task not found",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks?previous=external-task-3",
				strings.NewReader(`
					{
						"title": "T2"
					}
				`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "fails to insert task",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks",
				strings.NewReader(`
					{
						"title": "T1"
					}
				`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailInsertTask = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "list tasks",
			req:  httptest.NewRequest(http.MethodGet, "/tasks/v1/lists/external-list-1/tasks", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "list tasks list not found",
			req:  httptest.NewRequest(http.MethodGet, "/tasks/v1/lists/external-list-2/tasks", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {},
			},
		},
		{
			name: "fails to list tasks",
			req:  httptest.NewRequest(http.MethodGet, "/tasks/v1/lists/external-list-1/tasks", http.NoBody),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailListTasks = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "patch task",
			req: httptest.NewRequest(
				http.MethodPatch,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1",
				strings.NewReader(`
				{
					"title": "T1 Updated",
					"notes": "new notes"
				}
			`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1 Updated",
						Notes: "new notes",
					},
				},
			},
		},
		{
			name: "patch task clear notes and due",
			req: httptest.NewRequest(
				http.MethodPatch,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1",
				strings.NewReader(`
				{
					"notes":null,
					"due":null
				}
			`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
							Notes: "old notes",
							Due:   "2024-01-01T00:00:00Z",
						},
					},
				}
			},
			wantStatus: http.StatusOK,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
						Notes: "",
						Due:   "",
					},
				},
			},
		},
		{
			name: "patch task list not found",
			req: httptest.NewRequest(
				http.MethodPatch,
				"/tasks/v1/lists/external-list-2/tasks/external-task-1",
				strings.NewReader(`
				{
					"title": "T1 Updated"
				}
			`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "patch task not found",
			req: httptest.NewRequest(
				http.MethodPatch,
				"/tasks/v1/lists/external-list-1/tasks/external-task-2",
				strings.NewReader(`
				{
					"title": "T1 Updated"
				}
			`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "fails to patch task",
			req: httptest.NewRequest(
				http.MethodPatch,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1",
				strings.NewReader(`
				{
					"title": "T1 Updated"
				}
			`),
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailPatchTask = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "move task",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1/move?"+
					"destinationTasklist=external-list-2&previous=external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
					{
						Id:    "external-list-2",
						Title: "L2",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
					"external-list-2": {
						{
							Id:    "external-task-2",
							Title: "T2",
						},
						{
							Id:    "external-task-3",
							Title: "T3",
						},
					},
				}
			},
			wantStatus: http.StatusNoContent,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
				{
					Id:    "external-list-2",
					Title: "L2",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {},
				"external-list-2": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
					{
						Id:    "external-task-1",
						Title: "T1",
					},
					{
						Id:    "external-task-3",
						Title: "T3",
					},
				},
			},
		},
		{
			name: "move task defaults to source list",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1/move?previous=external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
						{
							Id:    "external-task-2",
							Title: "T2",
						},
					},
				}
			},
			wantStatus: http.StatusNoContent,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "move task source list not found",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-3/tasks/external-task-1/move?"+
					"destinationTasklist=external-list-2&previous=external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
					{
						Id:    "external-list-2",
						Title: "L2",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
					"external-list-2": {
						{
							Id:    "external-task-2",
							Title: "T2",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
				{
					Id:    "external-list-2",
					Title: "L2",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
				"external-list-2": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
				},
			},
		},
		{
			name: "move task source task not found",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks/external-task-3/move?"+
					"destinationTasklist=external-list-2&previous=external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
					{
						Id:    "external-list-2",
						Title: "L2",
					},
				}
				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
					"external-list-2": {
						{
							Id:    "external-task-2",
							Title: "T2",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
				{
					Id:    "external-list-2",
					Title: "L2",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
				"external-list-2": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
				},
			},
		},
		{
			name: "move task destination list not found",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1/move?"+
					"destinationTasklist=external-list-3&previous=external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
					{
						Id:    "external-list-2",
						Title: "L2",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
					"external-list-2": {
						{
							Id:    "external-task-2",
							Title: "T2",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
				{
					Id:    "external-list-2",
					Title: "L2",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
				"external-list-2": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
				},
			},
		},
		{
			name: "move task previous task not found",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1/move?"+
					"destinationTasklist=external-list-2&previous=external-task-3",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
					{
						Id:    "external-list-2",
						Title: "L2",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
					"external-list-2": {
						{
							Id:    "external-task-2",
							Title: "T2",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
				{
					Id:    "external-list-2",
					Title: "L2",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
				"external-list-2": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
				},
			},
		},
		{
			name: "fails to move task",
			req: httptest.NewRequest(
				http.MethodPost,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1/move?"+
					"destinationTasklist=external-list-2&previous=external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailMoveTask = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "delete task",
			req: httptest.NewRequest(
				http.MethodDelete,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
						{
							Id:    "external-task-2",
							Title: "T2",
						},
					},
				}
			},
			wantStatus: http.StatusNoContent,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-2",
						Title: "T2",
					},
				},
			},
		},
		{
			name: "delete task list not found",
			req: httptest.NewRequest(
				http.MethodDelete,
				"/tasks/v1/lists/external-list-2/tasks/external-task-1",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "delete task not found",
			req: httptest.NewRequest(
				http.MethodDelete,
				"/tasks/v1/lists/external-list-1/tasks/external-task-2",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.TaskLists = []*tasks.TaskList{
					{
						Id:    "external-list-1",
						Title: "L1",
					},
				}

				fake.Tasks = map[string][]*tasks.Task{
					"external-list-1": {
						{
							Id:    "external-task-1",
							Title: "T1",
						},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantTaskLists: []*tasks.TaskList{
				{
					Id:    "external-list-1",
					Title: "L1",
				},
			},
			wantTasks: map[string][]*tasks.Task{
				"external-list-1": {
					{
						Id:    "external-task-1",
						Title: "T1",
					},
				},
			},
		},
		{
			name: "fails to delete task",
			req: httptest.NewRequest(
				http.MethodDelete,
				"/tasks/v1/lists/external-list-1/tasks/external-task-1",
				http.NoBody,
			),
			setup: func(fake *FakeGoogleTasks) {
				fake.FailDeleteTask = true
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "unknown path not found",
			req: httptest.NewRequest(
				http.MethodGet,
				"/unknown/path/that/does/not/exist",
				http.NoBody,
			),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := NewFakeGoogleTasks(t)

			if tt.setup != nil {
				tt.setup(fake)
			}

			resp, err := fake.RoundTrip(tt.req)
			if err != nil {
				t.Fatalf("RoundTrip() unexpected error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("RoundTrip() status = %v, want %v", resp.StatusCode, tt.wantStatus)
			}

			cmpOpts := []cmp.Option{
				cmpopts.IgnoreFields(tasks.TaskList{}, "Updated"),
				cmpopts.IgnoreFields(tasks.Task{}, "Updated"),
				cmpopts.EquateEmpty(),
			}

			if diff := cmp.Diff(tt.wantTaskLists, fake.TaskLists, cmpOpts...); diff != "" {
				t.Errorf("TaskLists mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantTasks, fake.Tasks, cmpOpts...); diff != "" {
				t.Errorf("Tasks mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
