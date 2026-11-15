package ui

import "fmt"

// Err returns the first error retained by a constructor or With helper.
// New and Add report it when mounting a detached node. For mounted updates,
// use Set methods to receive each error immediately instead.
func (node *Node) Err() error { return node.constructionErr }

func (node *Node) withError(op string, err error) *Node {
	if node.constructionErr == nil && err != nil {
		node.constructionErr = fmt.Errorf("%s: %w", op, err)
	}

	return node
}

// WithTooltip sets supplementary help during construction. Err, New or Add
// reports the first construction error, even after later valid configuration.
func (node *Node) WithTooltip(text string) *Node {
	return node.withError("WithTooltip", node.SetTooltip(text))
}

// WithLabel sets the semantic name and description during construction.
func (node *Node) WithLabel(label, description string) *Node {
	return node.withError("WithLabel", node.SetLabel(label, description))
}

// WithButtonVariant sets an action's visual priority during construction.
func (node *Node) WithButtonVariant(variant ButtonVariant) *Node {
	return node.withError("WithButtonVariant", node.SetButtonVariant(variant))
}

// WithButtonVisual sets an action's icon or prepared image during construction.
func (node *Node) WithButtonVisual(visual ButtonVisual) *Node {
	return node.withError("WithButtonVisual", node.SetButtonVisual(visual))
}

// WithPanelBackground sets a prepared panel image or frame during construction.
func (node *Node) WithPanelBackground(background PanelBackground) *Node {
	return node.withError("WithPanelBackground", node.SetPanelBackground(background))
}

// WithChartOptions sets a chart's presentation during construction.
func (node *Node) WithChartOptions(options ChartOptions) *Node {
	return node.withError("WithChartOptions", node.SetChartOptions(options))
}

// WithHelpText sets a field's hint during construction.
func (node *Node) WithHelpText(text string) *Node {
	return node.withError("WithHelpText", node.SetHelpText(text))
}
