package ui

// reconcileTabs runs on structural/state mutations, never on an idle frame.
func (v *View) reconcileTabs() {
	for _, node := range v.nodes {
		if node.tabs == nil || node.view != v {
			continue
		}

		active := node.tabs.active
		if active >= 0 && node.tabAvailable(active) {
			continue
		}

		focused := withinNode(v.focus, node)

		target := node.nextAvailableTab(active)
		if target < 0 {
			node.clearTabs()

			continue
		}

		node.applyTab(target)

		if focused && node.view == v {
			v.SetFocus(node.tabs.buttons[target])
		}
	}
}

func (node *Node) nextAvailableTab(previous int) int {
	for step := 1; step <= len(node.tabs.buttons); step++ {
		index := (previous + step) % len(node.tabs.buttons)
		if node.tabAvailable(index) {
			return index
		}
	}

	return -1
}

func (node *Node) clearTabs() {
	if node.tabs.active < 0 {
		return
	}

	node.tabs.active = -1
	node.text = ""

	for index, button := range node.tabs.buttons {
		button.tabSkip, button.semanticSelected = true, false
		button.SetChecked(false)
		node.tabs.panels[index].SetHidden(true)
	}

	node.damage(true)
}
