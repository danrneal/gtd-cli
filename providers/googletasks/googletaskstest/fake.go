// Package googletaskstest provides an in-memory HTTP fake for the Google Tasks API.
package googletaskstest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"testing"
	"time"

	"google.golang.org/api/tasks/v1"
)

var (
	// Matches: /tasks/v1/users/@me/lists OR /tasks/v1/users/@me/lists/{taskListID}.
	taskListRegex = regexp.MustCompile(`^/tasks/v1/users/@me/lists(?:/([^/]+))?$`)
	// Matches: /tasks/v1/lists/{taskListID}/tasks OR /tasks/v1/lists/{taskListID}/tasks/{taskID}
	// OR /tasks/v1/lists/{taskListID}/tasks/{taskID}/move.
	taskRegex = regexp.MustCompile(`^/tasks/v1/lists/([^/]+)/tasks(?:/([^/]+)(?:/move)?)?$`)
)

// Fake provides a mock implementation of the Google Tasks API for testing purposes.
type Fake struct {
	t                  *testing.T
	TaskLists          []*tasks.TaskList
	Tasks              map[string][]*tasks.Task
	taskCounter        int
	FailInsertTaskList bool
	FailListTaskLists  bool
	FailPatchTaskList  bool
	FailDeleteTaskList bool
	FailInsertTask     bool
	FailListTasks      bool
	FailPatchTask      bool
	FailMoveTask       bool
	FailDeleteTask     bool
}

// NewFake initializes and returns a new Fake round tripper.
func NewFake(t *testing.T) *Fake {
	fake := &Fake{
		t:         t,
		TaskLists: []*tasks.TaskList{},
		Tasks:     map[string][]*tasks.Task{},
	}

	return fake
}

// RoundTrip intercepts HTTP requests and routes them to the appropriate mock handler.
func (f *Fake) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
	}

	if matches := taskListRegex.FindStringSubmatch(req.URL.Path); matches != nil {
		taskListID := matches[1]

		switch req.Method {
		case http.MethodPost:
			return f.insertTaskList(body), nil
		case http.MethodGet:
			return f.listTaskLists(), nil
		case http.MethodPatch:
			return f.patchTaskList(taskListID, body), nil
		case http.MethodDelete:
			return f.deleteTaskList(taskListID), nil
		}
	}

	if matches := taskRegex.FindStringSubmatch(req.URL.Path); matches != nil {
		taskListID := matches[1]
		taskID := matches[2]

		switch req.Method {
		case http.MethodPost:
			if taskID == "" {
				return f.insertTask(taskListID, req, body), nil
			}

			return f.moveTask(taskListID, taskID, req), nil
		case http.MethodGet:
			return f.listTasks(taskListID), nil
		case http.MethodPatch:
			return f.patchTask(taskListID, taskID, body), nil
		case http.MethodDelete:
			return f.deleteTask(taskListID, taskID), nil
		}
	}

	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewBufferString("Not Found: " + req.URL.Path)),
		Header:     make(http.Header),
	}

	return resp, nil
}

// insertTaskList simulates the creation of a new task list in Google Tasks.
func (f *Fake) insertTaskList(body []byte) *http.Response {
	if f.FailInsertTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	var taskList tasks.TaskList
	if err := json.Unmarshal(body, &taskList); err != nil {
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
			Header:     make(http.Header),
		}

		return resp
	}

	taskList.Id = fmt.Sprintf("external-list-%d", len(f.TaskLists)+1)
	taskList.Updated = time.Now().Format(time.RFC3339)
	f.TaskLists = append(f.TaskLists, &taskList)
	f.Tasks[taskList.Id] = []*tasks.Task{}

	respBody, err := json.Marshal(&taskList)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp
}

// listTaskLists simulates retrieving all task lists from Google Tasks.
func (f *Fake) listTaskLists() *http.Response {
	if f.FailListTaskLists {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	taskLists := &tasks.TaskLists{
		Items: f.TaskLists,
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

	return resp
}

// patchTaskList simulates updating an existing task list in Google Tasks.
func (f *Fake) patchTaskList(taskListID string, body []byte) *http.Response {
	if f.FailPatchTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	idx := slices.IndexFunc(f.TaskLists, func(t *tasks.TaskList) bool {
		return t.Id == taskListID
	})

	if idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	taskList := f.TaskLists[idx]
	if err := json.Unmarshal(body, &taskList); err != nil {
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
			Header:     make(http.Header),
		}

		return resp
	}

	taskList.Updated = time.Now().Format(time.RFC3339)

	respBody, err := json.Marshal(&taskList)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp
}

// deleteTaskList simulates deleting a task list from Google Tasks.
func (f *Fake) deleteTaskList(taskListID string) *http.Response {
	if f.FailDeleteTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	idx := slices.IndexFunc(f.TaskLists, func(t *tasks.TaskList) bool {
		return t.Id == taskListID
	})

	if idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	f.TaskLists = slices.Delete(f.TaskLists, idx, idx+1)
	delete(f.Tasks, taskListID)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
	}

	return resp
}

// insertTask simulates inserting a new task into a task list in Google Tasks.
func (f *Fake) insertTask(taskListID string, req *http.Request, body []byte) *http.Response {
	if f.FailInsertTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	taskItems, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	query := req.URL.Query()
	prevTaskID := query.Get("previous")
	idx := slices.IndexFunc(taskItems, func(t *tasks.Task) bool {
		return t.Id == prevTaskID
	})

	if prevTaskID != "" && idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	var task tasks.Task
	if err := json.Unmarshal(body, &task); err != nil {
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
			Header:     make(http.Header),
		}

		return resp
	}

	f.taskCounter++
	task.Id = fmt.Sprintf("external-task-%d", f.taskCounter)
	task.Updated = time.Now().Format(time.RFC3339)

	f.Tasks[taskListID] = slices.Insert(f.Tasks[taskListID], idx+1, &task)

	respBody, err := json.Marshal(&task)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp
}

// listTasks simulates retrieving all tasks for a given task list from Google Tasks.
func (f *Fake) listTasks(taskListID string) *http.Response {
	if f.FailListTasks {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	items, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	taskItems := &tasks.Tasks{
		Items: items,
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

	return resp
}

// patchTask simulates updating an existing task in Google Tasks.
func (f *Fake) patchTask(taskListID, taskID string, body []byte) *http.Response {
	if f.FailPatchTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	taskItems := f.Tasks[taskListID]
	idx := slices.IndexFunc(taskItems, func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	if idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	task := taskItems[idx]
	if err := json.Unmarshal(body, &task); err != nil {
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString("Bad Request")),
			Header:     make(http.Header),
		}

		return resp
	}

	if bytes.Contains(body, []byte(`"notes":null`)) {
		task.Notes = ""
	}

	if bytes.Contains(body, []byte(`"due":null`)) {
		task.Due = ""
	}

	task.Updated = time.Now().Format(time.RFC3339)

	respBody, err := json.Marshal(&task)
	if err != nil {
		f.t.Fatalf("failed to marshal response: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     make(http.Header),
	}

	return resp
}

// moveTask simulates changing the position of a task within or across task lists.
func (f *Fake) moveTask(taskListID, taskID string, req *http.Request) *http.Response {
	if f.FailMoveTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	srcTasks := f.Tasks[taskListID]
	taskIdx := slices.IndexFunc(srcTasks, func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	if taskIdx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	query := req.URL.Query()
	dstTaskListID := query.Get("destinationTasklist")
	if dstTaskListID == "" {
		dstTaskListID = taskListID
	}

	if _, ok := f.Tasks[dstTaskListID]; !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	task := srcTasks[taskIdx]
	f.Tasks[taskListID] = slices.Delete(f.Tasks[taskListID], taskIdx, taskIdx+1)

	prevTaskID := query.Get("previous")
	prevTaskIdx := slices.IndexFunc(f.Tasks[dstTaskListID], func(t *tasks.Task) bool {
		return t.Id == prevTaskID
	})

	if prevTaskID != "" && prevTaskIdx == -1 {
		f.Tasks[taskListID] = slices.Insert(f.Tasks[taskListID], taskIdx, task)
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	f.Tasks[dstTaskListID] = slices.Insert(f.Tasks[dstTaskListID], prevTaskIdx+1, task)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		Header:     make(http.Header),
	}

	return resp
}

// deleteTask simulates deleting a task from Google Tasks.
func (f *Fake) deleteTask(taskListID, taskID string) *http.Response {
	if f.FailDeleteTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp
	}

	taskItems := f.Tasks[taskListID]
	idx := slices.IndexFunc(taskItems, func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	if idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp
	}

	f.Tasks[taskListID] = slices.Delete(f.Tasks[taskListID], idx, idx+1)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
	}

	return resp
}
