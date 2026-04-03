package model_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/danrneal/gtd.nvim/internal/model"
)

func TestItem_Clean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		item      *model.Item
		wantTitle string
		wantDesc  string
		wantStat  model.Status
	}{
		{
			name: "title is trimmed and empty status defaults to not started",
			item: &model.Item{
				Title:  "  Buy Milk  \n",
				Status: "",
			},
			wantTitle: "Buy Milk",
			wantDesc:  "",
			wantStat:  model.StatusNotStarted,
		},
		{
			name: "standard markdown indentation with trailing whitespace",
			item: &model.Item{
				Title:       "Task",
				Description: "    First line  \n    Second line\t\n      Nested third line \n    Fourth line",
				Status:      model.StatusNotStarted,
			},
			wantTitle: "Task",
			wantDesc:  "First line\nSecond line\n  Nested third line\nFourth line",
			wantStat:  model.StatusNotStarted,
		},
		{
			name: "leading and trailing blank lines",
			item: &model.Item{
				Title: "Task",
				Description: `

  Description starts here
  And ends here
    
`,
				Status: model.StatusNotStarted,
			},
			wantTitle: "Task",
			wantDesc:  "Description starts here\nAnd ends here",
			wantStat:  model.StatusNotStarted,
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
		})
	}
}

func TestItem_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		item    *model.Item
		wantErr bool
	}{
		{
			name: "valid item",
			item: &model.Item{
				Title:  "Valid Task",
				ListID: "list-1",
				Status: model.StatusNotStarted,
			},
			wantErr: false,
		},
		{
			name: "invalid title",
			item: &model.Item{
				Title:  "",
				ListID: "list-1",
				Status: model.StatusNotStarted,
			},
			wantErr: true,
		},
		{
			name: "no list IDs",
			item: &model.Item{
				Title:  "Floating Task",
				Status: model.StatusNotStarted,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			item: &model.Item{
				Title:  "Valid Task",
				ListID: "list-1",
				Status: "invalid_status",
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
