package reorder

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/danrneal/gtd-cli/internal/model"
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
				ExternalID: new("L1"),
				Items: []*model.Item{
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("B"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("C"),
						ExternalListID: new("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: new("A")},
				{ExternalID: new("B")},
				{ExternalID: new("C")},
				{ExternalID: new("D")},
			},
			wantMoves: nil,
		},
		{
			name: "reorder to top (A moves to top)",
			updatedList: &model.List{
				ExternalID: new("L1"),
				Items: []*model.Item{
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("B"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("C"),
						ExternalListID: new("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: new("B")},
				{ExternalID: new("C")},
				{ExternalID: new("A")},
				{ExternalID: new("D")},
			},
			wantMoves: []Move{
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "",
				},
			},
		},
		{
			name: "reorder within list (C moves down)",
			updatedList: &model.List{
				ExternalID: new("L1"),
				Items: []*model.Item{
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("B"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("C"),
						ExternalListID: new("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: new("A")},
				{ExternalID: new("C")},
				{ExternalID: new("D")},
				{ExternalID: new("B")},
			},
			wantMoves: []Move{
				{
					ItemID:            "C",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "B",
				},
			},
		},
		{
			name: "initial sync (current items empty)",
			updatedList: &model.List{
				ExternalID: new("L1"),
				Items: []*model.Item{
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("B"),
						ExternalListID: new("L1"),
					},
				},
			},
			currentItems: []*model.Item{},
			wantMoves: []Move{
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "",
				},
				{
					ItemID:            "B",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "A",
				},
			},
		},
		{
			name: "move from other list / insert (B is new)",
			updatedList: &model.List{
				ExternalID: new("L1"),
				Items: []*model.Item{
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("B"),
						ExternalListID: new("L2"),
					},
					{
						ExternalID:     new("C"),
						ExternalListID: new("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: new("A")},
				{ExternalID: new("C")},
				{ExternalID: new("D")},
			},
			wantMoves: []Move{
				{
					ItemID:            "B",
					SourceListID:      "L2",
					DestinationListID: "L1",
					PreviousItemID:    "A",
				},
			},
		},
		{
			name: "duplicate items in updated list",
			updatedList: &model.List{
				ExternalID: new("L1"),
				Items: []*model.Item{
					{
						ExternalID:     new("C"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("D"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("E"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("A"),
						ExternalListID: new("L1"),
					},
					{
						ExternalID:     new("B"),
						ExternalListID: new("L1"),
					},
				},
			},
			currentItems: []*model.Item{
				{ExternalID: new("A")},
				{ExternalID: new("B")},
				{ExternalID: new("C")},
				{ExternalID: new("D")},
				{ExternalID: new("E")},
			},
			wantMoves: []Move{
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "E",
				},
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "A",
				},
				{
					ItemID:            "A",
					SourceListID:      "L1",
					DestinationListID: "L1",
					PreviousItemID:    "A",
				},
				{
					ItemID:            "B",
					SourceListID:      "L1",
					DestinationListID: "L1",
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
