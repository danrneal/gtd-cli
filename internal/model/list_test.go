package model

import (
	"testing"
	"time"
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
						ID:       "1",
						Title:    "Done task",
						Status:   StatusDone,
						Position: 0,
					},
					{
						ID:       "2",
						Title:    "Not started 1",
						Status:   StatusNotStarted,
						Position: 1,
					},
					{
						ID:       "3",
						Title:    "In progress 2",
						Status:   StatusInProgress,
						Position: 2,
					},
					{
						ID:       "4",
						Title:    "In progress 1",
						Status:   StatusInProgress,
						Position: 3,
					},
					{
						ID:       "5",
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
						ID:       "3",
						Title:    "In progress 2",
						Status:   StatusInProgress,
						Position: 0,
					},
					{
						ID:       "4",
						Title:    "In progress 1",
						Status:   StatusInProgress,
						Position: 1,
					},
					{
						ID:       "2",
						Title:    "Not started 1",
						Status:   StatusNotStarted,
						Position: 2,
					},
					{
						ID:       "5",
						Title:    "Not started 2",
						Status:   StatusNotStarted,
						Position: 3,
					},
					{
						ID:       "1",
						Title:    "Done task",
						Status:   StatusDone,
						Position: 4,
					},
				},
			},
		},
		{
			name: "Waiting For list sorts by Status then Created date",
			list: &List{
				Name:   ListWaitingFor,
				Status: StatusOpen,
				Items: []*Item{
					{
						ID:       "1",
						Title:    "Task 1 (Done)",
						Status:   StatusDone,
						Created:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						Position: 0,
					},
					{
						ID:       "2",
						Title:    "Task 2 (NotStarted, Newest)",
						Status:   StatusNotStarted,
						Created:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
						Position: 1,
					},
					{
						ID:       "3",
						Title:    "Task 3 (InProgress)",
						Status:   StatusInProgress,
						Created:  time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
						Position: 2,
					},
					{
						ID:       "4",
						Title:    "Task 4 (NotStarted, Oldest)",
						Status:   StatusNotStarted,
						Created:  time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
						Position: 3,
					},
				},
			},
			want: &List{
				Name:   ListWaitingFor,
				Status: StatusOpen,
				Items: []*Item{
					{
						ID:       "3",
						Title:    "Task 3 (InProgress)",
						Status:   StatusInProgress,
						Created:  time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
						Position: 0,
					},
					{
						ID:       "2",
						Title:    "Task 2 (NotStarted, Newest)",
						Status:   StatusNotStarted,
						Created:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
						Position: 1,
					},
					{
						ID:       "4",
						Title:    "Task 4 (NotStarted, Oldest)",
						Status:   StatusNotStarted,
						Created:  time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
						Position: 2,
					},
					{
						ID:       "1",
						Title:    "Task 1 (Done)",
						Status:   StatusDone,
						Created:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						Position: 3,
					},
				},
			},
		},
		{
			name: "Snoozed list sorts by Status then Snoozed date with nil handling",
			list: &List{
				Name:   ListSnoozed,
				Status: StatusOpen,
				Items: []*Item{
					{
						ID:       "a",
						Title:    "Task A (Done, Snoozed early)",
						Status:   StatusDone,
						Snoozed:  iso8601ToDate("2024-01-01"),
						Position: 0,
					},
					{
						ID:       "b",
						Title:    "Task B (NotStarted, Snoozed nil)",
						Status:   StatusNotStarted,
						Snoozed:  nil,
						Position: 1,
					},
					{
						ID:       "c",
						Title:    "Task C (InProgress, Snoozed late)",
						Status:   StatusInProgress,
						Snoozed:  iso8601ToDate("2024-01-10"),
						Position: 2,
					},
					{
						ID:       "d",
						Title:    "Task D (NotStarted, Snoozed early)",
						Status:   StatusNotStarted,
						Snoozed:  iso8601ToDate("2024-01-05"),
						Position: 3,
					},
					{
						ID:       "e",
						Title:    "Task E (NotStarted, Snoozed nil again)",
						Status:   StatusNotStarted,
						Snoozed:  nil,
						Position: 4,
					},
					{
						ID:       "f",
						Title:    "Task F (NotStarted, Snoozed later)",
						Status:   StatusNotStarted,
						Snoozed:  iso8601ToDate("2024-01-06"),
						Position: 5,
					},
				},
			},
			want: &List{
				Name:   ListSnoozed,
				Status: StatusOpen,
				Items: []*Item{
					{
						ID:       "c",
						Title:    "Task C (InProgress, Snoozed late)",
						Status:   StatusInProgress,
						Snoozed:  iso8601ToDate("2024-01-10"),
						Position: 0,
					},
					{
						ID:       "b",
						Title:    "Task B (NotStarted, Snoozed nil)",
						Status:   StatusNotStarted,
						Snoozed:  nil,
						Position: 1,
					},
					{
						ID:       "e",
						Title:    "Task E (NotStarted, Snoozed nil again)",
						Status:   StatusNotStarted,
						Snoozed:  nil,
						Position: 2,
					},
					{
						ID:       "d",
						Title:    "Task D (NotStarted, Snoozed early)",
						Status:   StatusNotStarted,
						Snoozed:  iso8601ToDate("2024-01-05"),
						Position: 3,
					},
					{
						ID:       "f",
						Title:    "Task F (NotStarted, Snoozed later)",
						Status:   StatusNotStarted,
						Snoozed:  iso8601ToDate("2024-01-06"),
						Position: 4,
					},
					{
						ID:       "a",
						Title:    "Task A (Done, Snoozed early)",
						Status:   StatusDone,
						Snoozed:  iso8601ToDate("2024-01-01"),
						Position: 5,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.list.Clean()

			if tt.list.Name != tt.want.Name {
				t.Errorf("Clean() Name = %q, want %q", tt.list.Name, tt.want.Name)
			}

			if tt.list.Status != tt.want.Status {
				t.Errorf("Clean() Status = %q, want %q", tt.list.Status, tt.want.Status)
			}

			if len(tt.list.Items) != len(tt.want.Items) {
				t.Fatalf("Clean() items length = %d, want %d", len(tt.list.Items), len(tt.want.Items))
			}

			for i, wantItem := range tt.want.Items {
				gotItem := tt.list.Items[i]
				if gotItem.ID != wantItem.ID {
					t.Errorf("Clean() item[%d] ID = %q, want %q", i, gotItem.ID, wantItem.ID)
				}

				if gotItem.Position != wantItem.Position {
					t.Errorf("Clean() item[%d] Position = %d, want %d", i, gotItem.Position, wantItem.Position)
				}
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
		ExternalID: new("ext-1"),
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
			name: "equal when one ID is empty",
			list: baseList,
			other: &List{
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
			},
			want: true,
		},
		{
			name: "different IDs when both set",
			list: baseList,
			other: &List{
				ID:         "2",
				ExternalID: new("ext-1"),
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
				ExternalID: new("ext-2"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
			},
			want: false,
		},
		{
			name: "different ExternalIDs (one nil)",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: nil,
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
			},
			want: true,
		},
		{
			name: "different names",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
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
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusDeleted,
				Position:   0,
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
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{Title: "New Item"},
				},
			},
			want: false,
		},
		{
			name: "different item IDs when both set",
			list: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1"},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-2"},
				},
			},
			want: false,
		},
		{
			name: "different item IDs (one empty)",
			list: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: "item-1"},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{ID: ""},
				},
			},
			want: true,
		},
		{
			name: "different item ExternalIDs when both set",
			list: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:         "item-1",
						ExternalID: new("ext-item-1"),
					},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:         "item-1",
						ExternalID: new("ext-item-2"),
						Position:   0,
					},
				},
			},
			want: false,
		},
		{
			name: "different item ExternalIDs (one nil)",
			list: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:         "item-1",
						ExternalID: new("ext-item-1"),
					},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:         "item-1",
						ExternalID: nil,
						Position:   0,
					},
				},
			},
			want: true,
		},
		{
			name: "equal lists ignoring metadata",
			list: baseList,
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Modified:   time.Now().Add(time.Hour),
				Items:      []*Item{},
			},
			want: true,
		},
		{
			name: "equal when list has deleted items",
			list: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:     "item-1",
						Title:  "Active",
						Status: StatusNotStarted,
					},
					{
						ID:     "item-2",
						Title:  "Deleted",
						Status: StatusDeleted,
					},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:     "item-1",
						Title:  "Active",
						Status: StatusNotStarted,
					},
				},
			},
			want: true,
		},
		{
			name: "equal when other has deleted items",
			list: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:     "item-1",
						Title:  "Active",
						Status: StatusNotStarted,
					},
				},
			},
			other: &List{
				ID:         "1",
				ExternalID: new("ext-1"),
				Name:       "Inbox",
				Status:     StatusOpen,
				Position:   0,
				Items: []*Item{
					{
						ID:     "item-1",
						Title:  "Active",
						Status: StatusNotStarted,
					},
					{
						ID:     "item-3",
						Title:  "Also Deleted",
						Status: StatusDeleted,
					},
				},
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

func iso8601ToDate(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}
