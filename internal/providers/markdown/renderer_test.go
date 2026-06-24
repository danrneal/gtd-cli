package markdown

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/danrneal/gtd-cli/internal/model"
)

func TestRender(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		writer  io.Writer
		lists   []model.List
		want    string
		wantErr bool
	}{
		{
			name:    "empty input",
			writer:  &bytes.Buffer{},
			lists:   nil,
			want:    "",
			wantErr: false,
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
			want: `# Inbox (0)

`,
			wantErr: false,
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
			want: `# List One (0)

# List Two (0) {{list-123}}

`,
			wantErr: false,
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
					},
				},
			},
			want: `# Statuses (3)
* [-] In progress
* [ ] Not started
* [x] ~~Done~~

`,
			wantErr: false,
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
							Title:     "Task",
							Position:  0,
							Status:    model.StatusNotStarted,
							Modified:  modified,
							ProjectID: new("proj-123"),
						},
					},
				},
			},
			want: `# Action (1)
* [ ] Task +proj-123

`,
			wantErr: false,
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
							Due:      iso8601ToDate("2024-01-02"),
						},
					},
				},
			},
			want: `# Action (1)
* [ ] Task due:2024-01-02

`,
			wantErr: false,
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
							Snoozed:  iso8601ToDate("2024-01-01"),
						},
					},
				},
			},
			want: `# Action (1)
* [ ] Task snoozed:2024-01-01

`,
			wantErr: false,
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
			want: `# Action (1)
* [ ] Task #tag1 #tag2

`,
			wantErr: false,
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
			want: `# Waiting For (1)
* [ ] Alice - Send report - 2024-01-02

`,
			wantErr: false,
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
			want: `# Action (1)
* [ ] Task {{item-456}}

`,
			wantErr: false,
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
			want: `# Notes (1)
* [ ] Task with description
First line of description.
Second line.

`,
			wantErr: false,
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
			wantErr: true,
		},
		{
			name:    "writer error",
			writer:  errWriter{},
			lists:   []model.List{{Name: "Test List"}},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := render(tt.writer, tt.lists)

			if (err != nil) != tt.wantErr {
				t.Fatalf("render() error = %v, wantErr %v", err, tt.wantErr)
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

func (errWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("simulated I/O error")
}
