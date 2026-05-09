package model

import (
	"testing"
	"time"
)

func TestList_Clean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		list     *List
		wantName string
		wantStat Status
	}{
		{
			name: "name is trimmed",
			list: &List{
				Name:   "  Inbox  \n",
				Status: StatusOpen,
			},
			wantName: "Inbox",
			wantStat: StatusOpen,
		},
		{
			name: "empty status defaults to open",
			list: &List{
				Name:   "Projects",
				Status: "",
			},
			wantName: "Projects",
			wantStat: StatusOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.list.Clean()

			if tt.list.Name != tt.wantName {
				t.Errorf("Clean() name = %q, want %q", tt.list.Name, tt.wantName)
			}

			if tt.list.Status != tt.wantStat {
				t.Errorf("Clean() status = %q, want %q", tt.list.Status, tt.wantStat)
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
