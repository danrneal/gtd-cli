package markdown

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/danrneal/gtd-cli/model"
)

func TestRender(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		writer  io.Writer
		lists   []model.List
		want    string
		wantErr error
	}{
		{
			name:   "empty input",
			writer: &bytes.Buffer{},
			lists:  nil,
			want:   "",
		},
		{
			name:   "basic list with no items",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			want: trimIndent(`
				# Inbox (0)

			`),
		},
		{
			name:   "multiple lists with mixed headers",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "List One",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
				{
					ID:       "list-123",
					Name:     "List Two",
					Position: 1,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			want: trimIndent(`
				# List One (0)
				
				# List Two (0) {{list-123}}

			`),
		},
		{
			name:   "item statuses",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Statuses",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Not started",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							Title:    "In progress",
							Position: 1,
							Status:   model.StatusInProgress,
							Modified: modified,
						},
						{
							Title:    "Done",
							Position: 2,
							Status:   model.StatusDone,
							Modified: modified,
						},
						{
							Title:    "Empty status",
							Position: 3,
							Status:   "",
							Modified: modified,
						},
					},
				},
			},
			want: trimIndent(`
				# Statuses (3)
				* [ ] Not started
				* [-] In progress
				* [ ] Empty status
				* [x] ~~Done~~

			`),
		},
		{
			name:   "active item count excludes done items",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Completed",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Done 1",
							Position: 0,
							Status:   model.StatusDone,
							Modified: modified,
						},
						{
							Title:    "Done 2",
							Position: 1,
							Status:   model.StatusDone,
							Modified: modified,
						},
					},
				},
			},
			want: trimIndent(`
				# Completed (0)
				* [x] ~~Done 1~~
				* [x] ~~Done 2~~

			`),
		},
		{
			name:   "item with project",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Action",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:      "Task",
							Position:   0,
							Status:     model.StatusNotStarted,
							Modified:   modified,
							ProjectTag: new("proj-123"),
						},
					},
				},
			},
			want: trimIndent(`
				# Action (1)
				* [ ] Task +proj-123

			`),
		},
		{
			name:   "item with due date",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Action",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
							Due:      iso8601ToDate(t, "2024-01-02"),
						},
					},
				},
			},
			want: trimIndent(`
				# Action (1)
				* [ ] Task due:2024-01-02

			`),
		},
		{
			name:   "item with snoozed date",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Action",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
							Snoozed:  iso8601ToDate(t, "2024-01-01"),
						},
					},
				},
			},
			want: trimIndent(`
				# Action (1)
				* [ ] Task snoozed:2024-01-01

			`),
		},
		{
			name:   "item with tags",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Action",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
							Tags:     []string{"tag1", "tag2"},
						},
					},
				},
			},
			want: trimIndent(`
				# Action (1)
				* [ ] Task #tag1 #tag2

			`),
		},
		{
			name:   "waiting for item formatting",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Waiting For",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:     "Send report",
							WaitingOn: "Alice",
							Position:  0,
							Status:    model.StatusNotStarted,
							Modified:  modified,
							Created:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
						},
					},
				},
			},
			want: trimIndent(`
				# Waiting For (1)
				* [ ] Alice - Send report - 2024-01-02

			`),
		},
		{
			name:   "item with ID",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Action",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-456",
							Title:    "Task",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			want: trimIndent(`
				# Action (1)
				* [ ] Task {{item-456}}

			`),
		},
		{
			name:   "multiline descriptions",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name:     "Notes",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:       "Task with description",
							Position:    0,
							Status:      model.StatusNotStarted,
							Modified:    modified,
							Description: "First line of description.\nSecond line.",
						},
					},
				},
			},
			want: trimIndent(`
				# Notes (1)
				* [ ] Task with description
				    First line of description.
				    Second line.

			`),
		},
		{
			name:   "invalid item status",
			writer: &bytes.Buffer{},
			lists: []model.List{
				{
					Name: "Invalid Status List",
					Items: []*model.Item{
						{Title: "Bad Task", Status: "unknown_status", ID: "item-1"},
					},
				},
			},
			want:    "",
			wantErr: cmpopts.AnyError,
		},
		{
			name:    "writer error",
			writer:  errWriter{},
			lists:   []model.List{{Name: "Test List"}},
			want:    "",
			wantErr: io.ErrShortWrite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := render(tt.writer, tt.lists)

			opts := []cmp.Option{
				cmpopts.EquateErrors(),
			}

			if diff := cmp.Diff(tt.wantErr, err, opts...); diff != "" {
				t.Fatalf("render() error mismatch (-want +got):\n%s", diff)
			}

			buf, ok := tt.writer.(*bytes.Buffer)
			if !ok {
				return
			}

			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("render() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// errWriter implements [io.Writer] and always returns an error for testing write failures.
type errWriter struct{}

// Write always returns [io.ErrShortWrite] to simulate a write failure.
func (errWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrShortWrite
}
