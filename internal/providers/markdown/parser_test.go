package markdown

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/danrneal/gtd.nvim/internal/model"
)

func TestParse(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		reader  io.Reader
		want    []model.List
		wantErr bool
	}{
		{
			name:   "empty input",
			reader: strings.NewReader(""),
			want:   nil,
		},
		{
			name: "file with no lists",
			reader: strings.NewReader(`
				This is just a file with some random text.
				But no headers at all.
			`),
			want: nil,
		},
		{
			name: "basic list with no items",
			reader: strings.NewReader(`
				# Inbox
			`),
			want: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
		},
		{
			name: "multiple lists",
			reader: strings.NewReader(`
				# List One

				# List Two
			`),
			want: []model.List{
				{
					Name:     "List One",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
				{
					Name:     "List Two",
					Position: 1,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
		},
		{
			name: "basic list with item",
			reader: strings.NewReader(`
				# Inbox
				* [ ] A simple task
			`),
			want: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "A simple task",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "multiline descriptions",
			reader: strings.NewReader(`
				# Notes
				* [ ] Task with description
				First line of description.
				Second line.
			`),
			want: []model.List{
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
		},
		{
			name: "multiple lists with items",
			reader: strings.NewReader(`
				# List One
				* [ ] Item One A

				# List Two
				* [ ] Item Two A
			`),
			want: []model.List{
				{
					Name:     "List One",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Item One A",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
				{
					Name:     "List Two",
					Position: 1,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Item Two A",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "items ignored if no preceding list",
			reader: strings.NewReader(`
				* [ ] Stray item
				# Inbox
				* [ ] Valid item
			`),
			want: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Valid item",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "list with external ID, count suffix, and multiple items",
			reader: strings.NewReader(`
				## Inbox (3) {{list-123}}
				* [ ] First task
				* [ ] Second task
				* [ ] Third task
			`),
			want: []model.List{
				{
					ID:       "list-123",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "First task",
							ListID:   "list-123",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							Title:    "Second task",
							ListID:   "list-123",
							Position: 1,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							Title:    "Third task",
							ListID:   "list-123",
							Position: 2,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "item statuses and strikethrough",
			reader: strings.NewReader(`
				# Statuses
				* [ ] Not started
				- [-] In progress
				- [~] In progress custom
				* [x] ~Done lowercase~
				* [X] ~~Done uppercase~~
			`),
			want: []model.List{
				{
					Name:     "Statuses",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "In progress",
							Position: 0,
							Status:   model.StatusInProgress,
							Modified: modified,
						},
						{
							Title:    "In progress custom",
							Position: 1,
							Status:   model.StatusInProgress,
							Modified: modified,
						},
						{
							Title:    "Not started",
							Position: 2,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							Title:    "Done lowercase",
							Position: 3,
							Status:   model.StatusDone,
							Modified: modified,
						},
						{
							Title:    "Done uppercase",
							Position: 4,
							Status:   model.StatusDone,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "item metadata parsing",
			reader: strings.NewReader(`
				# Action {{list-1}}
				* [ ] Complex task +P due:2024-01-02 snoozed:2024-01-01 #t #tag2 {{item-456}}
			`),
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Action",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:             "item-456",
							ExternalListID: stringPtr("list-1"),
							ListID:         "list-1",
							Title:          "Complex task",
							Position:       0,
							Status:         model.StatusNotStarted,
							Modified:       modified,
							ProjectID:      stringPtr("P"),
							Due:            iso8601ToDate("2024-01-02"),
							Snoozed:        iso8601ToDate("2024-01-01"),
							Tags:           []string{"t", "tag2"},
						},
					},
				},
			},
		},
		{
			name: "metadata syntax ignoring standalone plus symbol",
			reader: strings.NewReader(`
				# Normal
				* [ ] A + B
			`),
			want: []model.List{
				{
					Name:     "Normal",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "A + B",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "metadata syntax ignoring standalone hash symbol",
			reader: strings.NewReader(`
				# Normal
				* [ ] C # D
			`),
			want: []model.List{
				{
					Name:     "Normal",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "C # D",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "invalid snoozed date ignores prefix and appends to title",
			reader: strings.NewReader(`
				# Errors
				* [ ] Bad snoozed snoozed:tomorrow
			`),
			want: []model.List{
				{
					Name:     "Errors",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Bad snoozed snoozed:tomorrow",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "invalid due date ignores prefix and appends to title",
			reader: strings.NewReader(`
				# Errors
				* [ ] Bad due due:ASAP
			`),
			want: []model.List{
				{
					Name:     "Errors",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Bad due due:ASAP",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "waiting for list special behavior",
			reader: strings.NewReader(`
				# Waiting For
				* [ ] Alice - Send report
				* [ ] Bob - Review PR
			`),
			want: []model.List{
				{
					Name:     "Waiting For",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:     "Send report",
							WaitingOn: stringPtr("Alice"),
							Position:  0,
							Status:    model.StatusNotStarted,
							Modified:  modified,
							Created:   modified,
						},
						{
							Title:     "Review PR",
							WaitingOn: stringPtr("Bob"),
							Position:  1,
							Status:    model.StatusNotStarted,
							Modified:  modified,
							Created:   modified,
						},
					},
				},
			},
		},
		{
			name: "waiting for list ignores item without hyphen separator",
			reader: strings.NewReader(`
				# Waiting For
				* [ ] Just a task
			`),
			want: []model.List{
				{
					Name:     "Waiting For",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Just a task",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
		},
		{
			name: "waiting for item with incorrectly formatted date",
			reader: strings.NewReader(`
				# Waiting For
				* [ ] Bob - Review PR - Urgent
			`),
			want: []model.List{
				{
					Name:     "Waiting For",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:     "Review PR",
							Position:  0,
							Status:    model.StatusNotStarted,
							WaitingOn: stringPtr("Bob"),
							Modified:  modified,
							Created:   modified,
						},
					},
				},
			},
		},
		{
			name: "waiting for item with creation date",
			reader: strings.NewReader(`
				# Waiting For
				* [ ] Alice - Send budget report - May 15
			`),
			want: []model.List{
				{
					Name:     "Waiting For",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:     "Send budget report",
							Position:  0,
							Status:    model.StatusNotStarted,
							WaitingOn: stringPtr("Alice"),
							Created:   time.Date(0, time.May, 15, 0, 0, 0, 0, time.UTC),
							Modified:  modified,
						},
					},
				},
			},
		},
		{
			name:    "scanner error",
			reader:  errReader{},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parse(tt.reader, modified)

			if (err != nil) != tt.wantErr {
				t.Fatalf("parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// errReader implements [io.Reader] and always returns an error for testing read failures.
type errReader struct{}

func (errReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated I/O error")
}

func iso8601ToDate(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}
