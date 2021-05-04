package block

import (
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/tool"
)

// CoalBlock is a precious mineral block made from 9 coal.
type CoalBlock struct {
	solid
	bassDrum
}

// FlammabilityInfo ...
func (c CoalBlock) FlammabilityInfo() FlammabilityInfo {
	return FlammabilityInfo{
		Encouragement: 5,
		Flammability:  5,
	}
}

// BreakInfo ...
func (c CoalBlock) BreakInfo() BreakInfo {
	return BreakInfo{
		Hardness: 5,
		Harvestable: func(t tool.Tool) bool {
			return t.ToolType() == tool.TypePickaxe && t.HarvestLevel() >= tool.TierWood.HarvestLevel
		},
		Effective: pickaxeEffective,
		Drops:     simpleDrops(item.NewStack(c, 1)),
	}
}

// EncodeItem ...
func (CoalBlock) EncodeItem() (name string, meta int16) {
	return "minecraft:coal_block", 0
}

// EncodeBlock ...
func (CoalBlock) EncodeBlock() (name string, properties map[string]interface{}) {
	return "minecraft:coal_block", nil
}