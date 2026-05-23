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
	taskListRegex = regexp.MustCompile(`^/tasks/v1/users/@me/lists(?:/([^/]+))?$`)
	taskRegex     = regexp.MustCompile(`^/tasks/v1/lists/([^/]+)/tasks(?:/([^/]+)(?:/move)?)?$`)
)

type FakeGoogleTasks struct {
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

func NewFakeGoogleTasks(t *testing.T) *FakeGoogleTasks {
	googleTasks := &FakeGoogleTasks{
		t:         t,
		TaskLists: []*tasks.TaskList{},
		Tasks:     map[string][]*tasks.Task{},
	}

	return googleTasks
}

func (f *FakeGoogleTasks) RoundTrip(req *http.Request) (*http.Response, error) {
	if matches := taskListRegex.FindStringSubmatch(req.URL.Path); matches != nil {
		taskListID := matches[1]

		switch req.Method {
		case http.MethodPost:
			return f.InsertTaskList(req)
		case http.MethodGet:
			return f.ListTaskLists(req)
		case http.MethodPatch:
			return f.PatchTaskList(taskListID, req)
		case http.MethodDelete:
			return f.DeleteTaskList(taskListID, req)
		}
	}

	if matches := taskRegex.FindStringSubmatch(req.URL.Path); matches != nil {
		taskListID := matches[1]
		taskID := matches[2]

		switch req.Method {
		case http.MethodPost:
			if taskID == "" {
				return f.InsertTask(taskListID, req)
			}

			return f.MoveTask(taskListID, taskID, req)
		case http.MethodGet:
			return f.ListTasks(taskListID, req)
		case http.MethodPatch:
			return f.PatchTask(taskListID, taskID, req)
		case http.MethodDelete:
			return f.DeleteTask(taskListID, taskID, req)
		}
	}

	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewBufferString("Not Found: " + req.URL.Path)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) InsertTaskList(req *http.Request) (*http.Response, error) {
	if f.FailInsertTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	var taskList tasks.TaskList
	if err = json.Unmarshal(body, &taskList); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
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

	return resp, nil
}

func (f *FakeGoogleTasks) ListTaskLists(req *http.Request) (*http.Response, error) {
	if f.FailListTaskLists {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
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

	return resp, nil
}

func (f *FakeGoogleTasks) PatchTaskList(taskListID string, req *http.Request) (*http.Response, error) {
	if f.FailPatchTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
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

		return resp, nil
	}

	taskList := f.TaskLists[idx]
	if err = json.Unmarshal(body, &taskList); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
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

	return resp, nil
}

func (f *FakeGoogleTasks) DeleteTaskList(taskListID string, req *http.Request) (*http.Response, error) {
	if f.FailDeleteTaskList {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
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

		return resp, nil
	}

	f.TaskLists = slices.Delete(f.TaskLists, idx, idx+1)
	delete(f.Tasks, taskListID)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) InsertTask(taskListID string, req *http.Request) (*http.Response, error) {
	if f.FailInsertTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	taskItems, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	q := req.URL.Query()
	prevTaskID := q.Get("previous")
	idx := slices.IndexFunc(taskItems, func(t *tasks.Task) bool {
		return t.Id == prevTaskID
	})

	if prevTaskID != "" && idx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	var task tasks.Task
	if err = json.Unmarshal(body, &task); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
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

	return resp, nil
}

func (f *FakeGoogleTasks) ListTasks(taskListID string, req *http.Request) (*http.Response, error) {
	if f.FailListTasks {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	items, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
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

	return resp, nil
}

func (f *FakeGoogleTasks) PatchTask(taskListID, taskID string, req *http.Request) (*http.Response, error) {
	if f.FailPatchTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		f.t.Fatalf("failed to read request body: %v", err)
	}

	taskItems, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	idx := slices.IndexFunc(taskItems, func(t *tasks.Task) bool {
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

	task := taskItems[idx]
	if err = json.Unmarshal(body, &task); err != nil {
		f.t.Fatalf("failed to unmarshal request body: %v", err)
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

	return resp, nil
}

func (f *FakeGoogleTasks) MoveTask(taskListID, taskID string, req *http.Request) (*http.Response, error) {
	if f.FailMoveTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	srcTasks, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	taskIdx := slices.IndexFunc(srcTasks, func(t *tasks.Task) bool {
		return t.Id == taskID
	})

	if taskIdx == -1 {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	q := req.URL.Query()
	dstTaskListID := q.Get("destinationTasklist")
	if dstTaskListID == "" {
		dstTaskListID = taskListID
	}

	_, ok = f.Tasks[dstTaskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	task := srcTasks[taskIdx]
	f.Tasks[taskListID] = slices.Delete(f.Tasks[taskListID], taskIdx, taskIdx+1)

	prevTaskID := q.Get("previous")
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

		return resp, nil
	}

	f.Tasks[dstTaskListID] = slices.Insert(f.Tasks[dstTaskListID], prevTaskIdx+1, task)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		Header:     make(http.Header),
	}

	return resp, nil
}

func (f *FakeGoogleTasks) DeleteTask(taskListID, taskID string, req *http.Request) (*http.Response, error) {
	if f.FailDeleteTask {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal"}`)),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	taskItems, ok := f.Tasks[taskListID]
	if !ok {
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
			Header:     make(http.Header),
		}

		return resp, nil
	}

	idx := slices.IndexFunc(taskItems, func(t *tasks.Task) bool {
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

	f.Tasks[taskListID] = slices.Delete(f.Tasks[taskListID], idx, idx+1)

	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(bytes.NewBufferString("")),
		Header:     make(http.Header),
	}

	return resp, nil
}
