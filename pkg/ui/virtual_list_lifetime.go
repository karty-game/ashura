package ui

import "errors"

// Retiring a recycled item must not move an edit, held press, popup or stale
// semantic action onto a different logical item. Fresh IDs invalidate old targets.
func (v *View) retireVirtualRow(list, root *Node) error {
	if withinVirtualRow(v.focus, root) {
		// Keep keyboard navigation on the list without revealing the entire
		// viewport through an outer scroller while placement is in progress.
		v.retireVirtualFocus()
		v.focus = list
		list.damage(false)
	}

	if withinVirtualRow(v.hover, root) {
		v.hover = nil
		v.resumeTooltip()
	}

	if withinVirtualRow(v.popup, root) {
		v.closePopup()
	}

	if v.capture != nil && withinVirtualRow(v.capture.node, root) {
		if v.capture.scrolling && v.capture.scroller == list {
			v.capture.node = list
			v.pressed = nil
		} else {
			v.retireVirtualCapture()
		}
	}

	if v.virtualNotificationCount > 0 {
		return errVirtualLayoutRestart
	}

	root.virtualInactive = true

	v.reconcileModals()
	v.recycleVirtualIDs(root)

	return nil
}

func withinVirtualRow(node, root *Node) bool {
	for parent := node; parent != nil; parent = parent.parent {
		if parent == root {
			return true
		}
	}

	return false
}

func (v *View) recycleVirtualIDs(node *Node) {
	v.cancelControlRipple(node)
	node.motion = controlMotion{owner: v, listed: node.motion.listed}
	delete(v.nodes, node.id)
	v.next++
	node.id = v.next

	v.nodes[node.id] = node
	if node.kind == Input {
		node.undo = node.undo[:0]
		node.redo = node.redo[:0]
		node.composing = false
		node.anchor, node.cursor = 0, 0
		node.editOffset = 0
	}

	node.virtualIndex, node.virtualSize = 0, 0
	for _, child := range node.children {
		v.recycleVirtualIDs(child)
	}
}

// Virtual-row retirement happens during placement. Notify application callbacks
// before rebinding. Restart placement after notifying application callbacks, so
// callbacks may safely remove the list or its ancestor with their old row identity.
type virtualNotification struct {
	cancel func()
	change func(Change)
	value  Change
}

func (v *View) retireVirtualCapture() {
	node := v.capture.node
	callback := node.cancel
	node.cancel = nil

	v.cancelCapture()

	node.cancel = callback
	if callback != nil {
		v.virtualNotifications[v.virtualNotificationCount] = virtualNotification{cancel: callback}
		v.virtualNotificationCount++
	}
}

func (v *View) retireVirtualFocus() {
	node := v.focus
	callback := node.change
	composing := node.composing
	node.change = nil

	v.SetFocus(nil)

	node.change = callback
	if composing && callback != nil {
		v.virtualNotifications[v.virtualNotificationCount] = virtualNotification{change: callback, value: node.changed()}
		v.virtualNotificationCount++
	}
}

func (v *View) flushVirtualNotifications() {
	notifications := v.virtualNotifications
	count := v.virtualNotificationCount
	clear(v.virtualNotifications[:])
	v.virtualNotificationCount = 0

	for _, notification := range notifications[:count] {
		if notification.cancel != nil {
			notification.cancel()
		}

		if notification.change != nil {
			notification.change(notification.value)
		}
	}
}

func (v *View) layoutWithVirtualRetirement() error {
	for range 4 {
		err := v.layout()
		if err == nil {
			return nil
		}

		if !errors.Is(err, errVirtualLayoutRestart) {
			return err
		}

		v.layoutDirty = true
		v.flushVirtualNotifications()
	}

	return ErrReentrant
}
