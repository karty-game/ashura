package ui

// Semantic is the meaningful control projection, independent of paint parts.
type Semantic struct {
	PosInSet, SetSize                                               int
	Invalid                                                         bool
	DescribedBy                                                     ID
	Current                                                         string
	Controls, LabelledBy                                            ID
	Orientation                                                     string
	ID, Parent                                                      ID
	Role, Name, Description, Text                                   string
	Bounds                                                          Rect
	Value, Minimum, Maximum                                         float64
	Checked, Mixed, Selected, Expanded, Disabled, ReadOnly, Focused bool
}

// Semantics explicitly snapshots accessible structure; no OS/DOM bridge is implied.
func (v *View) Semantics() []Semantic {
	result := make([]Semantic, 0, len(v.order))
	for _, node := range v.order {
		if node.id == 0 || node.hidden || node.kind == Image {
			continue
		}

		role := semanticRole(node)

		name := node.label
		if name == "" {
			name = node.text
		}

		semantic := Semantic{
			ID:          node.id,
			Invalid:     node.invalid,
			Current:     node.current,
			Role:        role,
			Name:        name,
			Description: node.description,
			Text:        node.text,
			Bounds:      node.rect,
			Value:       node.value,
			Minimum:     node.minimum,
			Maximum:     node.maximum,
			Checked:     node.checked,
			Selected:    node.semanticSelected,
			Mixed:       node.mixed,
			Expanded:    v.popup == node,
			Disabled:    !v.inScope(node) || inheritedDisabled(node),
			ReadOnly:    node.readOnly,
			Focused:     v.focus == node,
		}
		if semantic.Description == "" {
			semantic.Description = node.tooltip
		}

		if node.describedBy != nil && node.describedBy.view == v {
			semantic.DescribedBy = node.describedBy.id
		}

		if node.controls != nil && node.controls.view == v {
			semantic.Controls = node.controls.id
		}

		if node.labelledBy != nil && node.labelledBy.view == v {
			semantic.LabelledBy = node.labelledBy.id
		}

		if role == "tablist" || node.kind == Separator {
			semantic.Orientation = "horizontal"
		}

		if node.parent != nil {
			semantic.Parent = node.parent.id
		}

		if node.kind == Select {
			semantic.Text = node.Selected()
		}

		semantic.PosInSet, semantic.SetSize = node.virtualIndex, node.virtualSize
		chartSemantic(node, &semantic)
		result = append(result, semantic)
	}

	return result
}

func chartSemantic(node *Node, semantic *Semantic) {
	if node.kind != Chart {
		return
	}

	semantic.Text = node.chart.summary
	if semantic.Description == "" {
		semantic.Description = node.chart.summary
	}
}

func inheritedDisabled(node *Node) bool {
	for parent := node; parent != nil; parent = parent.parent {
		if parent.disabled {
			return true
		}
	}

	return false
}

func semanticRole(node *Node) string {
	if node.role != "" {
		return node.role
	}

	return [...]string{Panel: "group", Text: "text", Button: "button", Checkbox: "checkbox", Switch: "switch", ToggleButton: "button", Slider: "slider", Select: "combobox", Input: "textbox", Progress: "progressbar", Separator: "separator", Chart: "img"}[node.kind]
}
