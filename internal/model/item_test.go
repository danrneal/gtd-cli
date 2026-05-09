package model

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestItem_Clean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		item        *Item
		wantTitle   string
		wantDesc    string
		wantStat    Status
		wantCreated time.Time
	}{
		{
			name: "title is trimmed and empty status defaults to not started",
			item: &Item{
				Title:   "  Buy Milk  \n",
				Status:  "",
				Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantTitle:   "Buy Milk",
			wantDesc:    "",
			wantStat:    StatusNotStarted,
			wantCreated: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "standard markdown indentation with trailing whitespace",
			item: &Item{
				Title:       "Task",
				Description: "    First line  \n    Second line\t\n      Nested third line \n    Fourth line",
				Status:      StatusNotStarted,
				Created:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantTitle:   "Task",
			wantDesc:    "First line\nSecond line\n  Nested third line\nFourth line",
			wantStat:    StatusNotStarted,
			wantCreated: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "leading and trailing blank lines",
			item: &Item{
				Title: "Task",
				Description: `

  Description starts here
  And ends here
    
`,
				Status:  StatusNotStarted,
				Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantTitle:   "Task",
			wantDesc:    "Description starts here\nAnd ends here",
			wantStat:    StatusNotStarted,
			wantCreated: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "zero created defaults to modified",
			item: &Item{
				Title:    "Task",
				Status:   StatusNotStarted,
				Modified: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
			},
			wantTitle:   "Task",
			wantDesc:    "",
			wantStat:    StatusNotStarted,
			wantCreated: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.item.Clean()

			if tt.item.Title != tt.wantTitle {
				t.Errorf("Clean() title = %q, want %q", tt.item.Title, tt.wantTitle)
			}

			if diff := cmp.Diff(tt.wantDesc, tt.item.Description); diff != "" {
				t.Errorf("Clean() description mismatch (-want +got):\n%s", diff)
			}

			if tt.item.Status != tt.wantStat {
				t.Errorf("Clean() status = %q, want %q", tt.item.Status, tt.wantStat)
			}

			if !tt.item.Created.Equal(tt.wantCreated) {
				t.Errorf("Clean() created = %v, want %v", tt.item.Created, tt.wantCreated)
			}
		})
	}
}

func TestItem_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		item    *Item
		wantErr bool
	}{
		{
			name: "valid item",
			item: &Item{
				Title:    "Valid Task",
				ListID:   "list-1",
				Status:   StatusNotStarted,
				Modified: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid title",
			item: &Item{
				Title:    "",
				ListID:   "list-1",
				Status:   StatusNotStarted,
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "no list IDs",
			item: &Item{
				Title:    "Floating Task",
				Status:   StatusNotStarted,
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			item: &Item{
				Title:    "Valid Task",
				ListID:   "list-1",
				Status:   "invalid_status",
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing modified timestamp",
			item: &Item{
				Title:  "Valid Task",
				ListID: "list-1",
				Status: StatusNotStarted,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.item.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestItem_Equal(t *testing.T) {
	t.Parallel()

	baseItem := &Item{
		ID:             "1",
		ExternalID:     stringPtr("ext-1"),
		ListID:         "list-1",
		ExternalListID: stringPtr("ext-1"),
		Title:          "Task",
		Description:    "Desc",
		Status:         StatusOpen,
		Position:       0,
		ProjectID:      stringPtr("proj-1"),
		WaitingOn:      stringPtr("person-1"),
		Snoozed:        iso8601ToDate("2024-01-01"),
		Due:            iso8601ToDate("2024-01-01"),
		Tags:           []string{"tag1"},
		Modified:       time.Now(),
		Created:        time.Now(),
	}

	tests := []struct {
		name  string
		item  *Item
		other *Item
		want  bool
	}{
		{
			name:  "nil other",
			item:  baseItem,
			other: nil,
			want:  false,
		},
		{
			name: "different IDs when both set",
			item: baseItem,
			other: &Item{
				ID:             "2",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different ExternalIDs when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-2"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Titles",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Different Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Descriptions",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Different Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Statuses",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusDone,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different ProjectID pointers (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      nil,
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different ProjectIDs when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-2"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different WaitingOn pointers (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      nil,
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different WaitingOn when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-2"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Snoozed times (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        nil,
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Snoozed times when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-02"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Due time pointers (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            nil,
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Due times when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-02"),
				Tags:           []string{"tag1"},
			},
			want: false,
		},
		{
			name: "different Tags (different length)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1", "tag2"},
			},
			want: false,
		},
		{
			name: "different Tags (same length, different items)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag2"},
			},
			want: false,
		},
		{
			name: "equal items with nil pointers",
			item: &Item{
				ID:             "1",
				ExternalID:     nil,
				ListID:         "list-1",
				ExternalListID: nil,
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      nil,
				WaitingOn:      nil,
				Snoozed:        nil,
				Due:            nil,
				Tags:           []string{"tag1"},
			},
			other: &Item{
				ID:             "1",
				ExternalID:     nil,
				ListID:         "list-1",
				ExternalListID: nil,
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      nil,
				WaitingOn:      nil,
				Snoozed:        nil,
				Due:            nil,
				Tags:           []string{"tag1"},
			},
			want: true,
		},
		{
			name: "equal items ignoring metadata",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     stringPtr("ext-1"),
				ListID:         "list-1",
				ExternalListID: stringPtr("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      stringPtr("proj-1"),
				WaitingOn:      stringPtr("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Modified:       time.Now().Add(time.Hour), // Different Modified
				Created:        time.Now().Add(time.Hour), // Different Created
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.item.Equal(tt.other)
			if got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func iso8601ToDate(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}
