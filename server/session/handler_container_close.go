package session

import (
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// ContainerCloseHandler handles the ContainerClose packet.
type ContainerCloseHandler struct{}

// Handle ...
func (h *ContainerCloseHandler) Handle(p packet.Packet, s *Session, tx *world.Tx, c Controllable) error {
	pk := p.(*packet.ContainerClose)

	c.MoveItemsToInventory()
	switch pk.WindowID {
	case 0:
		// Closing of the normal inventory.
		s.writePacket(&packet.ContainerClose{})
		s.invOpened = false
	case byte(s.openedWindowID.Load()):
		s.closeCurrentContainer(tx)
	case 0xff:
		// TODO: Handle closing the crafting grid.
	default:
		s.conf.Log.Debug("unexpected close request for unopened container", "window_id", pk.WindowID)
	}
	return nil
}
