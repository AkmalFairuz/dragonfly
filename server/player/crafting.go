package player

import (
	"fmt"
	"math"
	"slices"

	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/crafting"
	"github.com/df-mc/dragonfly/server/item/recipe"
)

// craftInventorySource is a snapshot of inventory slots consulted while preparing an auto-craft.
type craftInventorySource struct {
	source crafting.Source
	offset int
	stacks []item.Stack
}

// PrepareCraft prepares a crafting plan for a recipe crafted directly from the player's current crafting grid.
func (p *Player) PrepareCraft(craft recipe.Recipe, times int) (crafting.Plan, error) {
	if err := validateCraftingRecipe(craft); err != nil {
		return crafting.Plan{}, err
	}
	if times < 1 {
		return crafting.Plan{}, fmt.Errorf("times crafted must be at least 1")
	}

	offset, size := p.session().CraftingGridBounds()
	consumed := make([]bool, size)
	plan := crafting.Plan{
		Action: crafting.Action{
			Inputs:      make([]item.Stack, 0, len(craft.Input())),
			Repetitions: times,
		},
		Changes: make([]crafting.SlotChange, 0, len(craft.Input())),
	}

	for _, expected := range craft.Input() {
		var processed bool
		for slot := offset; slot < offset+size; slot++ {
			if consumed[slot-offset] {
				continue
			}
			has, _ := p.ui.Item(slot)
			if has.Empty() != expected.Empty() || has.Count() < expected.Count()*times || !matchingCraftItems(has, expected) {
				continue
			}

			processed, consumed[slot-offset] = true, true
			if removal := expected.Count() * times; removal > 0 {
				plan.Inputs = append(plan.Inputs, has.Grow(removal-has.Count()))
				plan.Changes = append(plan.Changes, crafting.SlotChange{
					Source: crafting.GridSource,
					Slot:   slot,
					Before: has,
					After:  has.Grow(-removal),
				})
			}
			break
		}
		if !processed {
			return crafting.Plan{}, fmt.Errorf("recipe could not consume expected item: %v", expected)
		}
	}
	plan.Outputs = repeatCraftStacks(craft.Output(), times)
	return p.approveCraftingPlan(plan)
}

// PrepareAutoCraft prepares a crafting plan for a recipe using the player's crafting grid and main inventory.
func (p *Player) PrepareAutoCraft(craft recipe.Recipe, times int) (crafting.Plan, error) {
	offset, size := p.session().CraftingGridBounds()
	grid := make([]item.Stack, size)
	for index := range grid {
		grid[index], _ = p.ui.Item(offset + index)
	}
	plan, err := planAutoCraft(craft, times, []craftInventorySource{
		{source: crafting.GridSource, offset: offset, stacks: grid},
		{source: crafting.InventorySource, stacks: p.inv.Slots()},
	})
	if err != nil {
		return crafting.Plan{}, err
	}
	return p.approveCraftingPlan(plan)
}

// planAutoCraft prepares an auto-crafting plan from inventory snapshots.
func planAutoCraft(craft recipe.Recipe, times int, sources []craftInventorySource) (crafting.Plan, error) {
	if err := validateCraftingRecipe(craft); err != nil {
		return crafting.Plan{}, err
	}
	if times < 1 {
		return crafting.Plan{}, fmt.Errorf("times crafted must be at least 1")
	}

	flattenedInputs := make([]recipe.Item, 0, len(craft.Input()))
	for _, ingredient := range craft.Input() {
		if ingredient.Empty() {
			continue
		}
		if index := slices.IndexFunc(flattenedInputs, func(other recipe.Item) bool {
			return matchingCraftItems(other, ingredient)
		}); index >= 0 {
			flattenedInputs[index] = growRecipeItem(ingredient, flattenedInputs[index].Count())
			continue
		}
		flattenedInputs = append(flattenedInputs, ingredient)
	}

	plan := crafting.Plan{
		Action: crafting.Action{
			Inputs:      make([]item.Stack, 0, len(flattenedInputs)),
			Repetitions: times,
		},
		Changes: make([]crafting.SlotChange, 0, len(flattenedInputs)),
	}
	type slotKey struct {
		source crafting.Source
		slot   int
	}
	changed := make(map[slotKey]int)

	for _, expected := range flattenedInputs {
		remaining := expected.Count() * times
		for sourceIndex := range sources {
			source := &sources[sourceIndex]
			for index, has := range source.stacks {
				if has.Empty() || !matchingCraftItems(has, expected) {
					continue
				}

				removal := min(remaining, has.Count())
				remaining -= removal
				after := has.Grow(-removal)
				source.stacks[index] = after
				plan.Inputs = append(plan.Inputs, has.Grow(removal-has.Count()))

				slot := source.offset + index
				key := slotKey{source: source.source, slot: slot}
				if changeIndex, ok := changed[key]; ok {
					plan.Changes[changeIndex].After = after
				} else {
					changed[key] = len(plan.Changes)
					plan.Changes = append(plan.Changes, crafting.SlotChange{
						Source: source.source,
						Slot:   slot,
						Before: has,
						After:  after,
					})
				}
				if remaining == 0 {
					break
				}
			}
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return crafting.Plan{}, fmt.Errorf("recipe could not consume expected item: %v", expected)
		}
	}
	plan.Outputs = repeatCraftStacks(craft.Output(), times)
	return plan, nil
}

// PrepareDynamicCraft prepares a crafting plan for the first matching server-side dynamic recipe in the player's grid.
func (p *Player) PrepareDynamicCraft(times int) (crafting.Plan, error) {
	if times < 1 {
		return crafting.Plan{}, fmt.Errorf("times crafted must be at least 1")
	}

	offset, size := p.session().CraftingGridBounds()
	input := make([]recipe.Item, size)
	for index := range input {
		stack, _ := p.ui.Item(offset + index)
		input[index] = stack
	}

	for _, dynamicRecipe := range recipe.DynamicRecipes() {
		if dynamicRecipe.Block() != "crafting_table" {
			continue
		}
		output, ok := dynamicRecipe.Match(input)
		if !ok {
			continue
		}

		minStackCount := math.MaxInt
		for index := range input {
			stack, _ := p.ui.Item(offset + index)
			if !stack.Empty() {
				minStackCount = min(minStackCount, stack.Count())
			}
		}
		times = min(times, minStackCount)

		plan := crafting.Plan{
			Action: crafting.Action{
				Inputs:      make([]item.Stack, 0, size),
				Outputs:     repeatCraftStacks(output, times),
				Repetitions: times,
			},
			Changes: make([]crafting.SlotChange, 0, size),
		}
		for index := range input {
			slot := offset + index
			stack, _ := p.ui.Item(slot)
			if stack.Empty() {
				continue
			}
			plan.Inputs = append(plan.Inputs, stack.Grow(times-stack.Count()))
			plan.Changes = append(plan.Changes, crafting.SlotChange{
				Source: crafting.GridSource,
				Slot:   slot,
				Before: stack,
				After:  stack.Grow(-times),
			})
		}
		return p.approveCraftingPlan(plan)
	}
	return crafting.Plan{}, fmt.Errorf("no matching recipe found for crafting grid")
}

// approveCraftingPlan runs the player's craft handler and returns the plan if it is allowed.
func (p *Player) approveCraftingPlan(plan crafting.Plan) (crafting.Plan, error) {
	ctx := newContext(p)
	p.Handler().HandleCraftingTable(ctx, crafting.Action{
		Inputs:      slices.Clone(plan.Inputs),
		Outputs:     slices.Clone(plan.Outputs),
		Repetitions: plan.Repetitions,
	})
	if ctx.Cancelled() {
		return crafting.Plan{}, fmt.Errorf("craft item was cancelled")
	}
	return plan, nil
}

// validateCraftingRecipe validates that the recipe is a normal crafting-table recipe supported by the player grid.
func validateCraftingRecipe(craft recipe.Recipe) error {
	_, shaped := craft.(recipe.Shaped)
	_, shapeless := craft.(recipe.Shapeless)
	if !shaped && !shapeless {
		return fmt.Errorf("recipe is not a shaped or shapeless recipe")
	}
	if craft.Block() != "crafting_table" {
		return fmt.Errorf("recipe is not a crafting table recipe")
	}
	return nil
}

// matchingCraftItems reports whether the two recipe items represent the same crafting ingredient.
func matchingCraftItems(has, expected recipe.Item) bool {
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

// repeatCraftStacks multiplies output stacks by the repetition count and splits them to respect max stack size.
func repeatCraftStacks(items []item.Stack, repetitions int) []item.Stack {
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

// growRecipeItem increases the count stored in a recipe item while preserving its concrete recipe item type.
func growRecipeItem(ingredient recipe.Item, count int) recipe.Item {
	switch ingredient := ingredient.(type) {
	case item.Stack:
		return ingredient.Grow(count)
	case recipe.ItemTag:
		return recipe.NewItemTag(ingredient.Tag(), ingredient.Count()+count)
	}
	panic(fmt.Errorf("unexpected recipe item %T", ingredient))
}
