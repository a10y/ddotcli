package main

import (
	"github.com/a10y/ddotcli/pkg/ddot"
	"fmt"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"time"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type cliModel struct {
	table      table.Model
	ddotClient ddot.Client
}
type updateCamerasMsg struct{}

func (c *cliModel) Init() tea.Cmd { return tea.ClearScreen }

func (c *cliModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return c, tea.Quit
		case tea.KeyEnter:
			// Get URL corresponding to the camera ID
			id := c.table.SelectedRow()[0]
			streamUrl := c.ddotClient.GetFfmpegUrl(id)

			exe := exec.Command("ffplay", "-i", streamUrl)
			devnull, _ := os.Open(os.DevNull)
			exe.Stdout = devnull
			exe.Stderr = devnull

			return c, tea.Sequence(
				tea.Printf("press ctrl-C to exit ffplay session"),
				tea.ExecProcess(exe, nil),
				tea.ClearScreen,
			)
		}
	case updateCamerasMsg:
		cameras := c.ddotClient.GetCameras()
		var rows []table.Row
		for _, camera := range cameras {
			rows = append(rows, toRow(camera))
		}
		c.table.SetRows(rows)
	}
	c.table, cmd = c.table.Update(msg)

	return c, cmd
}

func toRow(cameraInfo ddot.CameraInfo) table.Row {
	return []string{
		cameraInfo.Id,
		cameraInfo.Name,
		strconv.FormatFloat(float64(cameraInfo.Latitude), byte('f'), 4, 32),
		strconv.FormatFloat(float64(cameraInfo.Longitude), byte('f'), 4, 32),
	}
}

func (c *cliModel) View() string {
	var selectedId string
	if row := c.table.SelectedRow(); row == nil {
		selectedId = ""
	} else {
		selectedId = row[0]
	}
	idx := 1 + slices.IndexFunc(c.table.Rows(), func(row table.Row) bool {
		return row[0] == selectedId
	})

	darkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	return baseStyle.Render(c.table.View()) + "\n\n" +
		fmt.Sprintf("selected %v of %v", idx, len(c.table.Rows())) +
		"\n" +
		darkStyle.Render(fmt.Sprintf("press enter to open stream"))
}

var _ tea.Model = (*cliModel)(nil)

func main() {
	cctv, err := ddot.CreateClient()
	if err != nil {
		panic(err)
	}

	cols := []table.Column{
		{Title: "Id", Width: 32},
		{Title: "Name", Width: 60},
		{Title: "Latitude", Width: 20},
		{Title: "Longitude", Width: 20},
	}

	// Make a table
	tbl := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(50),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	tbl.SetStyles(s)

	app := tea.NewProgram(&cliModel{tbl, cctv})

	go func() {
		for {
			app.Send(updateCamerasMsg{})
			time.Sleep(1 * time.Second)
		}
	}()

	if _, err := app.Run(); err != nil {
		panic(fmt.Errorf("error: program run failed: %v", err))
	}
}
