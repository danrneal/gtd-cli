// Package reorder provides utilities to calculate moves.
package reorder

import "github.com/danrneal/gtd.nvim/internal/model"

// Move represents an operation to relocate or reorder an item.
// It supports both index-based positioning (Position) and linked-list positioning (PreviousItemID).
type Move struct {
	ItemID            string
	SourceListID      string
	DestinationListID string
	// Position is the 0-indexed position in the destination list.
	Position int
	// PreviousItemID is the ID of the item that should immediately precede this item in the destination list.
	// It is empty if the item should be the first in the list.
	PreviousItemID string
}

// CalculateMoves computes the minimal set of Move operations required to transform
// the current state of items (currentItems) into the desired state (list).
//
// Parameters:
//   - list: The target model.List containing the desired ordered Items.
//   - currentItems: The slice of items as they currently exist in the destination system.
//
// Returns:
//
//	A slice of Move operations. The sequence of moves assumes they are applied in order.
//	Positions are 0-indexed relative to the list at the time of the move.
//	It uses a Longest Common Subsequence (LCS) algorithm to minimize the number of moves by identifying stable items.
func CalculateMoves(list *model.List, currentItems []*model.Item) []Move {
	var moves []Move
	stableItemIDs := calculateStableItemIDs(list.Items, currentItems)
	previousItemID := ""
	for i, item := range list.Items {
		if _, ok := stableItemIDs[*item.ExternalID]; !ok {
			move := Move{
				ItemID:            *item.ExternalID,
				SourceListID:      *item.ExternalListID,
				DestinationListID: *list.ExternalID,
				Position:          i,
				PreviousItemID:    previousItemID,
			}

			moves = append(moves, move)
		}

		previousItemID = *item.ExternalID
	}

	return moves
}

func calculateStableItemIDs(updatedItems, currentItems []*model.Item) map[string]bool {
	currentItemIDs := make(map[string]int)
	for i, currentItem := range currentItems {
		currentItemIDs[*currentItem.ExternalID] = i
	}

	var currentItemIdxs []int
	for _, updatedItem := range updatedItems {
		if currentItemIdx, ok := currentItemIDs[*updatedItem.ExternalID]; ok {
			currentItemIdxs = append(currentItemIdxs, currentItemIdx)
		}
	}

	type node struct {
		currentItemIdx int
		length         int
		prevIdx        int
	}

	dp := make([]node, len(currentItemIdxs))
	maxLenIdx := -1
	for i, currentItemIdx := range currentItemIdxs {
		dp[i] = node{
			currentItemIdx: currentItemIdx,
			length:         1,
			prevIdx:        -1,
		}

		for j := range i {
			if currentItemIdxs[i] > currentItemIdxs[j] && dp[j].length >= dp[i].length {
				dp[i] = node{
					currentItemIdx: currentItemIdx,
					length:         dp[j].length + 1,
					prevIdx:        j,
				}
			}
		}

		if maxLenIdx == -1 || dp[i].length > dp[maxLenIdx].length {
			maxLenIdx = i
		}
	}

	stableItemIDs := make(map[string]bool)
	for maxLenIdx != -1 {
		node := dp[maxLenIdx]
		stableItemID := *currentItems[node.currentItemIdx].ExternalID
		stableItemIDs[stableItemID] = true
		maxLenIdx = node.prevIdx
	}

	return stableItemIDs
}
