package model

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestItem_Clean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		item *Item
		want *Item
	}{
		{
			name: "title is trimmed and empty status defaults to not started",
			item: &Item{
				Title:   "  Buy Milk  \n",
				Status:  "",
				Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			want: &Item{
				Title:   "Buy Milk",
				Status:  StatusNotStarted,
				Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "standard markdown indentation with trailing whitespace",
			item: &Item{
				Title:       "Task",
				Description: "    First line  \n    Second line\t\n      Nested third line \n      Nested fourth line",
				Status:      StatusNotStarted,
				Created:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			want: &Item{
				Title:       "Task",
				Description: "First line\nSecond line\n  Nested third line\n  Nested fourth line",
				Status:      StatusNotStarted,
				Created:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
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
			want: &Item{
				Title:       "Task",
				Description: "Description starts here\nAnd ends here",
				Status:      StatusNotStarted,
				Created:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "zero created defaults to modified",
			item: &Item{
				Title:    "Task",
				Status:   StatusNotStarted,
				Modified: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
			},
			want: &Item{
				Title:    "Task",
				Status:   StatusNotStarted,
				Modified: time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
				Created:  time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.item.Clean()

			if diff := cmp.Diff(tt.want, tt.item); diff != "" {
				t.Errorf("Clean() mismatch (-want +got):\n%s", diff)
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

func TestItem_Equivalent(t *testing.T) {
	t.Parallel()

	baseItem := &Item{
		ID:             "1",
		ExternalID:     new("ext-1"),
		ListID:         "list-1",
		ExternalListID: new("ext-1"),
		Title:          "Task",
		Description:    "Desc",
		Status:         StatusOpen,
		Position:       0,
		ProjectID:      new("proj-1"),
		WaitingOn:      new("person-1"),
		Snoozed:        iso8601ToDate("2024-01-01"),
		Due:            iso8601ToDate("2024-01-01"),
		Tags:           []string{"tag1"},
		Modified:       time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Created:        time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
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
			name: "equal when one ID is empty",
			item: baseItem,
			other: &Item{
				ExternalID:  new("ext-1"),
				Title:       "Task",
				Description: "Desc",
				Status:      StatusOpen,
				ProjectID:   new("proj-1"),
				WaitingOn:   new("person-1"),
				Snoozed:     iso8601ToDate("2024-01-01"),
				Due:         iso8601ToDate("2024-01-01"),
				Tags:        []string{"tag1"},
				Created:     baseItem.Created,
			},
			want: true,
		},
		{
			name: "equal when one ExternalID is nil",
			item: baseItem,
			other: &Item{
				ID:          "1",
				Title:       "Task",
				Description: "Desc",
				Status:      StatusOpen,
				ProjectID:   new("proj-1"),
				WaitingOn:   new("person-1"),
				Snoozed:     iso8601ToDate("2024-01-01"),
				Due:         iso8601ToDate("2024-01-01"),
				Tags:        []string{"tag1"},
				Created:     baseItem.Created,
			},
			want: true,
		},
		{
			name: "different IDs when both set",
			item: baseItem,
			other: &Item{
				ID:             "2",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different ExternalIDs when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-2"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Titles",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Different Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Descriptions",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Different Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Statuses",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusDone,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different ProjectID pointers (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      nil,
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different ProjectIDs when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-2"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different WaitingOn pointers (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      nil,
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different WaitingOn when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-2"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Snoozed times (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        nil,
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Snoozed times when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-02"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Due time pointers (one nil)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            nil,
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Due times when both set",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-02"),
				Tags:           []string{"tag1"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Tags (different length)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1", "tag2"},
				Created:        baseItem.Created,
			},
			want: false,
		},
		{
			name: "different Tags (same length, different items)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag2"},
				Created:        baseItem.Created,
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
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Modified:       time.Now().Add(time.Hour),
				Created:        baseItem.Created.AddDate(1, 0, 0).Add(time.Hour),
			},
			want: true,
		},
		{
			name: "not equal items (different created, has waiting on)",
			item: baseItem,
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      new("person-1"),
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Modified:       baseItem.Modified,
				Created:        time.Now().Add(24 * time.Hour),
			},
			want: false,
		},
		{
			name: "equal items (different created, no waiting on)",
			item: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      nil,
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Modified:       baseItem.Modified,
				Created:        baseItem.Created,
			},
			other: &Item{
				ID:             "1",
				ExternalID:     new("ext-1"),
				ListID:         "list-1",
				ExternalListID: new("ext-1"),
				Title:          "Task",
				Description:    "Desc",
				Status:         StatusOpen,
				Position:       0,
				ProjectID:      new("proj-1"),
				WaitingOn:      nil,
				Snoozed:        iso8601ToDate("2024-01-01"),
				Due:            iso8601ToDate("2024-01-01"),
				Tags:           []string{"tag1"},
				Modified:       baseItem.Modified,
				Created:        time.Now().Add(24 * time.Hour),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.item.Equivalent(tt.other)
			if got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}
