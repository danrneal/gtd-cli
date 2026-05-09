package model

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestList_Clean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		list *List
		want *List
	}{
		{
			name: "name is trimmed",
			list: &List{
				Name:   "  Inbox  \n",
				Status: StatusOpen,
			},
			want: &List{
				Name:   "Inbox",
				Status: StatusOpen,
			},
		},
		{
			name: "empty status defaults to open",
			list: &List{
				Name:   "Projects",
				Status: "",
			},
			want: &List{
				Name:   "Projects",
				Status: StatusOpen,
			},
		},
		{
			name: "default list sorting by status and position",
			list: &List{
				Name:   "Inbox",
				Status: StatusOpen,
				Items: []*Item{
					{
						Title:    "Done task",
						Status:   StatusDone,
						Position: 0,
					},
					{
						Title:    "Not started 1",
						Status:   StatusNotStarted,
						Position: 1,
					},
					{
						Title:    "In progress 2",
						Status:   StatusInProgress,
						Position: 2,
					},
					{
						Title:    "In progress 1",
						Status:   StatusInProgress,
						Position: 3,
					},
					{
						Title:    "Not started 2",
						Status:   StatusNotStarted,
						Position: 4,
					},
				},
			},
			want: &List{
				Name:   "Inbox",
				Status: StatusOpen,
				Items: []*Item{
					{
						Title:    "In progress 2",
						Status:   StatusInProgress,
						Position: 0,
					},
					{
						Title:    "In progress 1",
						Status:   StatusInProgress,
						Position: 1,
					},
					{
						Title:    "Not started 1",
						Status:   StatusNotStarted,
						Position: 2,
					},
					{
						Title:    "Not started 2",
						Status:   StatusNotStarted,
						Position: 3,
					},
					{
						Title:    "Done task",
						Status:   StatusDone,
						Position: 4,
					},
				},
			},
		},
		{
			name: "Waiting For list sorts by Created date",
			list: &List{
				Name:   ListWaitingFor,
				Status: StatusOpen,
				Items: []*Item{
					{
						Title:    "Task 1 (Newest)",
						Created:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
						Position: 0,
					},
					{
						Title:    "Task 2 (Oldest)",
						Created:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						Position: 1,
					},
					{
						Title:    "Task 3 (Middle)",
						Created:  time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
						Position: 2,
					},
				},
			},
			want: &List{
				Name:   ListWaitingFor,
				Status: StatusOpen,
				Items: []*Item{
					{
						Title:    "Task 2 (Oldest)",
						Created:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						Position: 0,
					},
					{
						Title:    "Task 3 (Middle)",
						Created:  time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
						Position: 1,
					},
					{
						Title:    "Task 1 (Newest)",
						Created:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
						Position: 2,
					},
				},
			},
		},
		{
			name: "Snoozed list sorts by Snoozed date with nil handling",
			list: &List{
				Name:   ListSnoozed,
				Status: StatusOpen,
				Items: []*Item{
					{
						Title:    "Task A (Snoozed late)",
						Snoozed:  iso8601ToDate("2024-01-10"),
						Position: 0,
					},
					{
						Title:    "Task B (Snoozed nil)",
						Snoozed:  nil,
						Position: 1,
					},
					{
						Title:    "Task C (Snoozed early)",
						Snoozed:  iso8601ToDate("2024-01-05"),
						Position: 2,
					},
					{
						Title:    "Task D (Snoozed nil again)",
						Snoozed:  nil,
						Position: 3,
					},
				},
			},
			want: &List{
				Name:   ListSnoozed,
				Status: StatusOpen,
				Items: []*Item{
					{
						Title:    "Task B (Snoozed nil)",
						Snoozed:  nil,
						Position: 0,
					},
					{
						Title:    "Task D (Snoozed nil again)",
						Snoozed:  nil,
						Position: 1,
					},
					{
						Title:    "Task C (Snoozed early)",
						Snoozed:  iso8601ToDate("2024-01-05"),
						Position: 2,
					},
					{
						Title:    "Task A (Snoozed late)",
						Snoozed:  iso8601ToDate("2024-01-10"),
						Position: 3,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.list.Clean()

			if diff := cmp.Diff(tt.want, tt.list); diff != "" {
				t.Errorf("Clean() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestList_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		list    *List
		wantErr bool
	}{
		{
			name: "valid list",
			list: &List{
				Name:     "Inbox",
				Status:   StatusOpen,
				Modified: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid name",
			list: &List{
				Name:     "",
				Status:   StatusOpen,
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			list: &List{
				Name:     "Inbox",
				Status:   "unknown_status",
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing modified timestamp",
			list: &List{
				Name:   "Inbox",
				Status: StatusOpen,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.list.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestList_Equal(t *testing.T) {
	t.Parallel()

	baseList := &List{
		ID:         "1",
		ExternalID: stringPtr("ext-1"),
		Name:       "Inbox",
		Status:     StatusOpen,
		Position:   0,
		Modified:   time.Now(),
		Items:      []*Item{},
	}

	tests := []struct {
		name  string
		list  *List
		other *List
		want  bool
	}{
		{
			name:  "nil other",
			list:  baseList,
			other: nil,
			want:  false,
		},
		{
			name: "different IDs when both set",
			list: baseList,
			other: &List{
				ID:         "2",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
			},
			want: false,
		},
		{
			name: "different ExternalIDs when both set",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-2"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
			},
			want: false,
		},
		{
			name: "different names",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Projects",
				Status:     StatusOpen,
				Position:   0,
			},
			want: false,
		},
		{
			name: "different statuses",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusDeleted,
				Position:   0,
			},
			want: false,
		},
		{
			name: "different positions",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   1,
			},
			want: false,
		},
		{
			name: "equal lists with nil pointers",
			list: &List{
				ID:         "1",
				ExternalID: nil,
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1"},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: nil,
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1"},
				},
			},
			want: true,
		},
		{
			name: "different item lengths",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items:      []*Item{{Title: "New Item"}},
			},
			want: false,
		},
		{
			name: "different item IDs when both set",
			list: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1", Position: 0},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-2", Position: 0},
				},
			},
			want: false,
		},
		{
			name: "different item ExternalIDs when both set",
			list: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1", ExternalID: stringPtr("ext-item-1"), Position: 0},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1", ExternalID: stringPtr("ext-item-2"), Position: 0},
				},
			},
			want: false,
		},
		{
			name: "equal lists ignoring metadata",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: stringPtr("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Modified:   time.Now().Add(time.Hour),
				Items:      []*Item{},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.list.Equal(tt.other)
			if got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
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
