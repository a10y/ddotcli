package main

import (
	"ai.intrinsiclabs.chatter/pkg/ddot"
	"fmt"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"os/exec"
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

func (c *cliModel) Init() tea.Cmd { return nil }

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
			return c, tea.Batch(
				tea.Printf("opening stream to %v", c.table.SelectedRow()[1]),
				tea.ExecProcess(exec.Command("ffplay", "-i", streamUrl), nil),
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
	// Return a table with all the table
	return baseStyle.Render(c.table.View()) + "\n"
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
