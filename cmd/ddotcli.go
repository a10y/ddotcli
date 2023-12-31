package main

import (
	"fmt"
	"github.com/a10y/ddotcli/pkg/ddot"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"io"
	"log"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type ActivePane uint8

const (
	PaneCameras ActivePane = iota
	PaneRecordings
)

type cliModel struct {
	camerasTable    table.Model
	recordingsTable table.Model
	ddotClient      ddot.Client
	activePane      ActivePane
	recordings      []recordingProcess
}

type recordingProcess struct {
	cameraId   string
	cameraName string
	outfile    string
	exe        *exec.Cmd
	// Wait for this routine to go.
	stopChan chan string
}

// Make a channel so that the managing goroutine will wait for all child processes to complete properly...

type updateCamerasMsg struct{}

func (c *cliModel) Init() tea.Cmd { return tea.ClearScreen }

func (c *cliModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			// Quit the program

			// Close all recordings
			for _, recordingProc := range c.recordings {
				log.Printf("halting recording %v...\n", recordingProc.outfile)
				recordingProc.stopChan <- "stop"
			}
			return c, tea.Quit
		case tea.KeyEnter:
			if c.activePane == PaneRecordings {
				break
			}

			id := c.camerasTable.SelectedRow()[0]
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
		case tea.KeyTab:
			if c.activePane == PaneCameras {
				c.activePane = PaneRecordings
				c.recordingsTable.Focus()
				c.camerasTable.Blur()
			} else {
				c.activePane = PaneCameras
				c.camerasTable.Focus()
				c.recordingsTable.Blur()
			}
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "r":
				cameraId := c.camerasTable.SelectedRow()[0]
				cameraName := c.camerasTable.SelectedRow()[1]

				outfile := fmt.Sprintf("%v.ts", cameraId)
				streamUrl := c.ddotClient.GetFfmpegUrl(cameraId)
				exe := exec.Command("ffmpeg", "-i", streamUrl, outfile)

				stopChan := make(chan string)
				recording := recordingProcess{
					cameraId, cameraName, outfile, exe, stopChan,
				}

				go func() {
					defer close(stopChan)

					stderr, err := exe.StderrPipe()
					if err != nil {
						panic("Failed to pipe to stdout of ffmpeg")
					}

					if err := exe.Start(); err != nil {
						buf, _ := io.ReadAll(stderr)
						panic(fmt.Errorf("error: failed to run recording: %v: stderr: %v", err, string(buf)))
					}

					select {
					case _ = <-stopChan:
						// kill -6
						if err = exe.Process.Signal(syscall.SIGABRT); err != nil {
							log.Printf("ffmpeg process kill -6 failed for %v, may become a zombie\n", outfile)
						}
						return
					}
				}()

				c.recordings = append(c.recordings, recording)
				var rows []table.Row
				for _, recording := range c.recordings {
					rows = append(rows, []string{recording.cameraName, recording.outfile})
				}

				c.recordingsTable.SetRows(rows)
			}
		}
	case updateCamerasMsg:
		cameras := c.ddotClient.GetCameras()
		var rows []table.Row
		for _, camera := range cameras {
			rows = append(rows, toRow(camera))
		}
		slices.SortFunc(rows, func(a, b table.Row) int {
			return strings.Compare(a[1], b[1])
		})

		c.camerasTable.SetRows(rows)
	}
	if c.activePane == PaneCameras {
		c.camerasTable, cmd = c.camerasTable.Update(msg)
	} else {
		c.recordingsTable, cmd = c.recordingsTable.Update(msg)
	}

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
	if row := c.camerasTable.SelectedRow(); row == nil {
		selectedId = ""
	} else {
		selectedId = row[0]
	}
	idx := 1 + slices.IndexFunc(c.camerasTable.Rows(), func(row table.Row) bool {
		return row[0] == selectedId
	})

	darkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	return baseStyle.Render(c.camerasTable.View()) + "\n\n" +
		baseStyle.Render(c.recordingsTable.View()) + "\n\n" +
		fmt.Sprintf("selected %v of %v", idx, len(c.camerasTable.Rows())) +
		"\n" +
		darkStyle.Render(fmt.Sprintf("press enter to open stream   press ctrl-C to quit    press r to begin recording"))
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

	// Make a camerasTable
	tbl := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(40),
	)

	recordingCols := []table.Column{
		{"Stream", 20},
		{"Size", 20},
	}
	recordingTable := table.New(
		table.WithColumns(recordingCols),
		table.WithFocused(false),
		table.WithHeight(15),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	tbl.SetStyles(s)

	app := tea.NewProgram(&cliModel{tbl, recordingTable, cctv, PaneCameras, []recordingProcess{}}, tea.WithMouseAllMotion())

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
