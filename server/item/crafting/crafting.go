// Package crafting holds values used to describe and apply crafting actions.
package crafting

import "github.com/df-mc/dragonfly/server/item"

// Source identifies an inventory that may supply crafting ingredients.
type Source uint8

const (
	// GridSource identifies the active 2x2 or 3x3 crafting grid.
	GridSource Source = iota
	// InventorySource identifies the player's main inventory.
	InventorySource
)

// Action describes a crafting action exposed to a player handler.
type Action struct {
	// Inputs contains snapshots of the concrete item stacks consumed by the action.
	Inputs []item.Stack
	// Outputs contains the item stacks produced by the action.
	Outputs []item.Stack
	// Repetitions is the number of times the recipe is crafted.
	Repetitions int
}

// Plan describes an approved crafting action and the slot changes needed to apply it.
type Plan struct {
	Action
	// Changes contains the inventory mutations needed to apply the action.
	Changes []SlotChange
}

// SlotChange describes one inventory mutation in a Plan.
type SlotChange struct {
	// Source identifies the inventory containing Slot.
	Source Source
	// Slot is the slot index within Source.
	Slot int
	// Before is the stack that must still occupy Slot when the plan is applied.
	Before item.Stack
	// After is the stack written to Slot when the plan is applied.
	After item.Stack
}
