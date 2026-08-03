package session

import (
	"fmt"

	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/crafting"
	"github.com/df-mc/dragonfly/server/item/creative"
	"github.com/df-mc/dragonfly/server/item/inventory"
	"github.com/df-mc/dragonfly/server/item/recipe"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// handleCraft handles the CraftRecipe request action.
func (h *ItemStackRequestHandler) handleCraft(a *protocol.CraftRecipeStackRequestAction, s *Session, tx *world.Tx, c Controllable) error {
	craft, ok := s.recipes[a.RecipeNetworkID]
	if !ok {
		// Try dynamic recipes if no static recipe matches
		plan, err := c.PrepareDynamicCraft(int(a.NumberOfCrafts))
		if err != nil {
			return err
		}
		return h.applyCraftingPlan(plan, s, tx)
	}
	plan, err := c.PrepareCraft(craft, int(a.NumberOfCrafts))
	if err != nil {
		return err
	}
	return h.applyCraftingPlan(plan, s, tx)
}

// handleAutoCraft handles the AutoCraftRecipe request action.
func (h *ItemStackRequestHandler) handleAutoCraft(a *protocol.AutoCraftRecipeStackRequestAction, s *Session, tx *world.Tx, c Controllable) error {
	craft, ok := s.recipes[a.RecipeNetworkID]
	if !ok {
		// Try dynamic recipes if no static recipe matches
		plan, err := c.PrepareDynamicCraft(int(a.TimesCrafted))
		if err != nil {
			return err
		}
		return h.applyCraftingPlan(plan, s, tx)
	}
	plan, err := c.PrepareAutoCraft(craft, int(a.TimesCrafted))
	if err != nil {
		return err
	}
	return h.applyCraftingPlan(plan, s, tx)
}

// handleCreativeCraft handles the CreativeCraft request action.
func (h *ItemStackRequestHandler) handleCreativeCraft(a *protocol.CraftCreativeStackRequestAction, s *Session, tx *world.Tx, c Controllable) error {
	if !c.GameMode().CreativeInventory() {
		return fmt.Errorf("can only craft creative items in gamemode creative/spectator")
	}
	index := a.CreativeItemNetworkID - 1
	if int(index) >= len(creative.Items()) {
		return fmt.Errorf("creative item with network ID %v does not exist", index)
	}
	it := creative.Items()[index].Stack
	it = it.Grow(it.MaxCount() - 1)
	return h.createResults(s, tx, it)
}

// matchingStacks reports whether two recipe items match in a crafting context.
func matchingStacks(has, expected recipe.Item) bool {
	switch expected := expected.(type) {
	case item.Stack:
		switch has := has.(type) {
		case recipe.ItemTag:
			name, _ := expected.Item().EncodeItem()
			return has.Contains(name)
		case item.Stack:
			_, variants := expected.Value("variants")
			if !variants {
				return has.Comparable(expected)
			}
			nameOne, _ := has.Item().EncodeItem()
			nameTwo, _ := expected.Item().EncodeItem()
			return nameOne == nameTwo
		}
		panic(fmt.Errorf("client has unexpected recipe item %T", has))
	case recipe.ItemTag:
		switch has := has.(type) {
		case item.Stack:
			name, _ := has.Item().EncodeItem()
			return expected.Contains(name)
		case recipe.ItemTag:
			return has.Tag() == expected.Tag()
		}
		panic(fmt.Errorf("client has unexpected recipe item %T", has))
	}
	panic(fmt.Errorf("tried to match with unexpected recipe item %T", expected))
}

// repeatStacks repeats item stacks, splitting outputs that exceed their maximum stack size.
func repeatStacks(items []item.Stack, repetitions int) []item.Stack {
	output := make([]item.Stack, 0, len(items))
	for _, stack := range items {
		count, maxCount := stack.Count(), stack.MaxCount()
		for total := count * repetitions; total > 0; {
			increase := min(total, maxCount)
			total -= increase
			output = append(output, stack.Grow(increase-count))
		}
	}
	return output
}

// applyCraftingPlan validates and applies a crafting plan, then creates its output items.
func (h *ItemStackRequestHandler) applyCraftingPlan(plan crafting.Plan, s *Session, tx *world.Tx) error {
	if err := s.validateCraftingPlan(plan); err != nil {
		return err
	}
	for _, change := range plan.Changes {
		info, _, _ := s.craftingSlot(change.Source, change.Slot)
		h.setItemInSlot(info, change.After, s, tx)
	}
	return h.createResults(s, tx, plan.Outputs...)
}

// validateCraftingPlan verifies every slot before any part of a plan is applied.
func (s *Session) validateCraftingPlan(plan crafting.Plan) error {
	type slotKey struct {
		source crafting.Source
		slot   int
	}
	seen := make(map[slotKey]struct{}, len(plan.Changes))
	for _, change := range plan.Changes {
		key := slotKey{source: change.Source, slot: change.Slot}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("crafting plan changes source %v slot %v more than once", change.Source, change.Slot)
		}
		seen[key] = struct{}{}

		_, inv, err := s.craftingSlot(change.Source, change.Slot)
		if err != nil {
			return err
		}
		current, err := inv.Item(change.Slot)
		if err != nil {
			return err
		}
		if !current.Equal(change.Before) {
			return fmt.Errorf("crafting inventory changed before plan could be applied")
		}
	}
	return nil
}

// craftingSlot resolves a crafting inventory slot to protocol and server inventory representations.
func (s *Session) craftingSlot(source crafting.Source, slot int) (protocol.StackRequestSlotInfo, *inventory.Inventory, error) {
	switch source {
	case crafting.GridSource:
		offset, size := s.CraftingGridBounds()
		if slot < offset || slot >= offset+size {
			return protocol.StackRequestSlotInfo{}, nil, fmt.Errorf("crafting grid slot %v out of range", slot)
		}
		return protocol.StackRequestSlotInfo{
			Container: protocol.FullContainerName{ContainerID: protocol.ContainerCraftingInput},
			Slot:      byte(slot),
		}, s.ui, nil
	case crafting.InventorySource:
		if _, err := s.inv.Item(slot); err != nil {
			return protocol.StackRequestSlotInfo{}, nil, err
		}
		return protocol.StackRequestSlotInfo{
			Container: protocol.FullContainerName{ContainerID: protocol.ContainerCombinedHotBarAndInventory},
			Slot:      byte(slot),
		}, s.inv, nil
	default:
		return protocol.StackRequestSlotInfo{}, nil, fmt.Errorf("unsupported crafting inventory %v", source)
	}
}
