//nolint:mnd // Sample chart ranges, datasets and presentation metrics are explicit.
package main

import (
	"fmt"
	"strconv"

	"github.com/karty-game/ashura/pkg/ui"
)

func chartShowcase(update func(string), check func(error)) *ui.Node {
	return ui.Column(
		card(
			ui.NewText("Charts for every screen").WithStyle(ui.Style{FontSize: 21}),
			ui.NewText(
				"Results, comparisons and debug samples use the same retained WebGPU layer. Change a dataset below to see it update.",
			),
		),
		ui.Row(frameChartShowcase(update, check), matchChartShowcase(update, check)).WithStyle(ui.Style{Wrap: true, Gap: 16}),
		ui.Row(rewardChartShowcase(update, check), buildChartShowcase(update, check)).WithStyle(ui.Style{Wrap: true, Gap: 16}),
	).WithStyle(ui.Style{Gap: 16})
}

func frameChartShowcase(update func(string), check func(error)) *ui.Node {
	theme := ui.DarkTheme()
	data := ui.ChartData{
		Labels: []string{"1", "2", "3", "4", "5", "6", "7", "8"},
		Series: []ui.ChartSeries{
			{Name: "CPU", Values: []float64{10, 12, 11, 17, 14, 12, 13, 11}, Color: theme.Accent},
			{Name: "GPU", Values: []float64{7, 8, 9, 12, 10, 8, 9, 8}, Color: theme.Info},
		},
	}
	chart := chartNode(ui.LineChart, "Frame time in milliseconds", data)
	chart.WithChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 0, Max: 33}, LineWidth: 2})

	samples := 8
	status := ui.NewText("8 samples · lower is better").WithStyle(ui.Style{FontSize: 12, Foreground: theme.Muted})
	advance := action("Sample frames", ui.SecondaryButton, func() {
		samples++
		for index := range data.Labels {
			data.Labels[index] = strconv.Itoa(samples - len(data.Labels) + index + 1)
		}

		for index := range data.Series {
			values := data.Series[index].Values
			copy(values, values[1:])
			values[len(values)-1] = float64(7 + (samples*7+index*11)%14)
		}

		check(chart.SetChartData(data))
		check(status.SetText(fmt.Sprintf("%d samples · lower is better", samples)))

		update("Frame chart sampled. Unchanged charts retain their GPU data.")
	})

	return chartCard("Frame budget", "Line · CPU and GPU time (ms)", chart, status, advance)
}

func matchChartShowcase(update func(string), check func(error)) *ui.Node {
	theme := ui.DarkTheme()
	match := 0
	data := func() ui.ChartData {
		values := [][]float64{{-8, 25, 46, 34}, {-15, 16, 29, 52}}
		if match%2 != 0 {
			values = [][]float64{{12, -6, 38, 62}, {5, 23, -11, 48}}
		}

		return ui.ChartData{
			Labels: []string{"R1", "R2", "R3", "R4"},
			Series: []ui.ChartSeries{
				{Name: "You", Values: values[0], Color: theme.Accent},
				{Name: "Rival", Values: values[1], Color: theme.Info},
			},
		}
	}
	chart := chartNode(ui.BarChart, "Net round score", data())
	chart.WithChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: -20, Max: 80}})

	status := ui.NewText("Match 1 · gains and penalties").WithStyle(ui.Style{FontSize: 12, Foreground: theme.Muted})
	next := action("Next match", ui.SecondaryButton, func() {
		match++

		check(chart.SetChartData(data()))
		check(status.SetText(fmt.Sprintf("Match %d · gains and penalties", match+1)))

		update("Match results changed. Negative bars extend below zero.")
	})

	return chartCard("Match results", "Bar · net points by round", chart, status, next)
}

func rewardChartShowcase(update func(string), check func(error)) *ui.Node {
	const rewardName = "Rewards"

	theme := ui.DarkTheme()
	data := ui.ChartData{
		Labels: []string{"Combat", "Loot", "Quests", "Bonus"},
		Series: []ui.ChartSeries{{Name: rewardName, Values: []float64{58, 16, 18, 8}}},
	}
	chart := chartNode(ui.PieChart, "Reward sources", data)
	chart.WithChartOptions(ui.ChartOptions{Palette: []ui.Color{theme.Accent, theme.Info, theme.Warning, theme.Danger}})
	status := ui.NewText("100 rewards · share by source").WithStyle(ui.Style{FontSize: 12, Foreground: theme.Muted})
	empty := action("Zero rewards", ui.SecondaryButton, func() {
		check(chart.SetChartData(ui.ChartData{
			Labels: data.Labels,
			Series: []ui.ChartSeries{{Name: rewardName, Values: []float64{0, 0, 0, 0}}},
		}))
		check(status.SetText("No rewards yet"))

		update("Zero-total pie displays its empty state.")
	})
	single := action("Combat only", ui.SecondaryButton, func() {
		check(chart.SetChartData(ui.ChartData{
			Labels: data.Labels,
			Series: []ui.ChartSeries{{Name: rewardName, Values: []float64{100, 0, 0, 0}}},
		}))
		check(status.SetText("100 combat rewards · one complete slice"))

		update("A single reward source fills the complete pie.")
	})
	restore := action("Restore rewards", ui.SecondaryButton, func() {
		check(chart.SetChartData(data))
		check(status.SetText("100 rewards · share by source"))

		update("Reward breakdown restored.")
	})

	return chartCard(rewardName, "Pie · reward sources", chart, status,
		ui.Row(empty, single, restore).WithStyle(ui.Style{Wrap: true, Gap: 8}))
}

func buildChartShowcase(update func(string), check func(error)) *ui.Node {
	theme := ui.DarkTheme()
	mage := false
	data := func() ui.ChartData {
		name, values := "Scout", []float64{65, 45, 90, 60, 75, 85}
		if mage {
			name, values = "Mage", []float64{85, 35, 55, 95, 60, 75}
		}

		return ui.ChartData{
			Labels: []string{"Power", "Guard", "Speed", "Focus", "Luck", "Range"},
			Series: []ui.ChartSeries{
				{Name: name, Values: values, Color: theme.Accent},
				{Name: "Balanced", Values: []float64{60, 60, 60, 60, 60, 60}, Color: theme.Info},
			},
		}
	}
	chart := chartNode(ui.RadarChart, "Character attributes", data())
	chart.WithChartOptions(ui.ChartOptions{Range: &ui.ChartRange{Min: 0, Max: 100}})

	status := ui.NewText("Scout build · attributes out of 100").WithStyle(ui.Style{FontSize: 12, Foreground: theme.Muted})
	switchBuild := action("Switch build", ui.SecondaryButton, func() {
		mage = !mage
		current := data()
		check(chart.SetChartData(current))
		check(status.SetText(current.Series[0].Name + " build · attributes out of 100"))

		update("Character comparison changed. The reference build stays visible.")
	})

	return chartCard("Character build", "Radar · compare six attributes", chart, status, switchBuild)
}

func chartNode(kind ui.ChartKind, label string, data ui.ChartData) *ui.Node {
	return ui.NewChart(kind, label, data).WithStyle(ui.Style{Height: 250, FontSize: 11})
}

func chartCard(title, subtitle string, children ...*ui.Node) *ui.Node {
	content := make([]*ui.Node, 0, 2+len(children))
	content = append(content,
		ui.NewText(title).WithStyle(ui.Style{FontSize: 18}),
		ui.NewText(subtitle).WithStyle(ui.Style{FontSize: 12, Foreground: ui.DarkTheme().Muted}),
	)
	content = append(content, children...)

	return card(content...).WithStyle(ui.Style{Basis: 360, Grow: 1, Padding: 20, Gap: 14, Fill: ui.DarkTheme().Surface, Radius: 12})
}
