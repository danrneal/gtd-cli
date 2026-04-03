package model_test

import (
	"testing"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
)

func TestList_Clean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		list     *model.List
		wantName string
		wantStat model.Status
	}{
		{
			name: "name is trimmed",
			list: &model.List{
				Name:   "  Inbox  \n",
				Status: model.StatusOpen,
			},
			wantName: "Inbox",
			wantStat: model.StatusOpen,
		},
		{
			name: "empty status defaults to open",
			list: &model.List{
				Name:   "Projects",
				Status: "",
			},
			wantName: "Projects",
			wantStat: model.StatusOpen,
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
		list    *model.List
		wantErr bool
	}{
		{
			name: "valid list",
			list: &model.List{
				Name:     "Inbox",
				Status:   model.StatusOpen,
				Modified: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid name",
			list: &model.List{
				Name:     "",
				Status:   model.StatusOpen,
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			list: &model.List{
				Name:     "Inbox",
				Status:   "unknown_status",
				Modified: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing modified timestamp",
			list: &model.List{
				Name:   "Inbox",
				Status: model.StatusOpen,
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
