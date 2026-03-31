package reorder

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/danrneal/gtd.nvim/internal/model"
)

func TestCalculateMoves(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		updatedList  *model.List
		currentItems []*model.Item
		wantMoves    []Move
	}{
		{
			name: "no change",
			updatedList: &model.List{
				ExternalID: stringPtr("L1"),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("A")},
				{ExternalID: stringPtr("B")},
				{ExternalID: stringPtr("C")},
				{ExternalID: stringPtr("D")},
			},
			wantMoves: nil,
		},
		{
			name: "reorder to top (A moves to top)",
			updatedList: &model.List{
				ExternalID: stringPtr("L1"),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("B")},
				{ExternalID: stringPtr("C")},
				{ExternalID: stringPtr("A")},
				{ExternalID: stringPtr("D")},
			},
			wantMoves: []Move{
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					Position:          0,
					PreviousItemID:    "",
				},
			},
		},
		{
			name: "reorder within list (C moves down)",
			updatedList: &model.List{
				ExternalID: stringPtr("L1"),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("A")},
				{ExternalID: stringPtr("C")},
				{ExternalID: stringPtr("D")},
				{ExternalID: stringPtr("B")},
			},
			wantMoves: []Move{
				{
					ItemID:            "C",
					SourceListID:      "L1",
					DestinationListID: "L1",
					Position:          2,
					PreviousItemID:    "B",
				},
			},
		},
		{
			name: "initial sync (current items empty)",
			updatedList: &model.List{
				ExternalID: stringPtr("L1"),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{},
			wantMoves: []Move{
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					Position:          0,
					PreviousItemID:    "",
				},
				{
					ItemID:            "B",
					SourceListID:      "L1",
					DestinationListID: "L1",
					Position:          1,
					PreviousItemID:    "A",
				},
			},
		},
		{
			name: "move from other list / insert (B is new)",
			updatedList: &model.List{
				ExternalID: stringPtr("L1"),
				Items: []*model.Item{
					{
						ExternalID:     stringPtr("A"),
						ExternalListID: stringPtr("L1"),
					},
					{
						ExternalID:     stringPtr("B"),
						ExternalListID: stringPtr("L2"),
					},
					{
						ExternalID:     stringPtr("C"),
						ExternalListID: stringPtr("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: stringPtr("A")},
				{ExternalID: stringPtr("C")},
				{ExternalID: stringPtr("D")},
			},
			wantMoves: []Move{
				{
					ItemID:            "B",
					SourceListID:      "L2",
					DestinationListID: "L1",
					Position:          1,
					PreviousItemID:    "A",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := CalculateMoves(tt.updatedList, tt.currentItems)
			if diff := cmp.Diff(tt.wantMoves, got); diff != "" {
				t.Errorf("CalculateMoves() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
