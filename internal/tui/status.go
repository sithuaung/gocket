package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AppStats struct {
	ID          string         `json:"id"`
	Connections int            `json:"connections"`
	Channels    int            `json:"channels"`
	TopChannels []ChannelStats `json:"top_channels,omitempty"`
}

type ChannelStats struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Subscribers int    `json:"subscribers"`
}

type StatsResponse struct {
	Uptime        string     `json:"uptime"`
	GoRoutines    int        `json:"goroutines"`
	MemAllocMB    float64    `json:"mem_alloc_mb"`
	MemSysMB      float64    `json:"mem_sys_mb"`
	TotalConns    int        `json:"total_connections"`
	TotalChannels int        `json:"total_channels"`
	Apps          []AppStats `json:"apps"`
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED")).
			PaddingBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#06B6D4"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9FAFB")).
			Bold(true)

	goodStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B"))

	appBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Padding(0, 1).
			MarginBottom(1)

	channelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)
)

type statsMsg StatsResponse
type errMsg error
type tickMsg time.Time

type Model struct {
	addr     string
	stats    *StatsResponse
	err      error
	quitting bool
}

func NewModel(addr string) Model {
	return Model{addr: addr}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchStats(m.addr), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}

	case statsMsg:
		s := StatsResponse(msg)
		m.stats = &s
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg
		m.stats = nil
		return m, nil

	case tickMsg:
		return m, tea.Batch(fetchStats(m.addr), tickCmd())
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ Gocket Status"))
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Connection error: %v", m.err)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  Trying %s...", m.addr)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Press q to quit"))
		b.WriteString("\n")
		return b.String()
	}

	if m.stats == nil {
		b.WriteString(dimStyle.Render("  Connecting..."))
		b.WriteString("\n")
		return b.String()
	}

	s := m.stats

	// Global stats
	fmt.Fprintf(&b, "  %s %s    %s %s    %s %s    %s %s\n",
		labelStyle.Render("Uptime:"),
		valueStyle.Render(s.Uptime),
		labelStyle.Render("Goroutines:"),
		valueStyle.Render(fmt.Sprintf("%d", s.GoRoutines)),
		labelStyle.Render("Mem:"),
		valueStyle.Render(fmt.Sprintf("%.1f MB", s.MemAllocMB)),
		labelStyle.Render("Sys:"),
		dimStyle.Render(fmt.Sprintf("%.1f MB", s.MemSysMB)),
	)

	fmt.Fprintf(&b, "  %s %s         %s %s\n\n",
		labelStyle.Render("Connections:"),
		connStyle(s.TotalConns),
		labelStyle.Render("Channels:"),
		valueStyle.Render(fmt.Sprintf("%d", s.TotalChannels)),
	)

	// Per-app stats
	for _, app := range s.Apps {
		var appContent strings.Builder
		fmt.Fprintf(&appContent, "%s\n", headerStyle.Render(fmt.Sprintf("App: %s", app.ID)))
		fmt.Fprintf(&appContent, "  %s %s    %s %s\n",
			labelStyle.Render("Connections:"),
			connStyle(app.Connections),
			labelStyle.Render("Channels:"),
			valueStyle.Render(fmt.Sprintf("%d", app.Channels)),
		)

		if len(app.TopChannels) > 0 {
			// Sort by subscriber count descending
			sorted := make([]ChannelStats, len(app.TopChannels))
			copy(sorted, app.TopChannels)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].Subscribers > sorted[j].Subscribers
			})

			fmt.Fprintf(&appContent, "\n  %s\n", labelStyle.Render("Channels:"))
			limit := min(len(sorted), 10)
			for _, ch := range sorted[:limit] {
				typeBadge := dimStyle.Render(fmt.Sprintf("[%s]", ch.Type))
				fmt.Fprintf(&appContent, "    %s %s  %s %s\n",
					channelStyle.Render(ch.Name),
					typeBadge,
					labelStyle.Render("subs:"),
					valueStyle.Render(fmt.Sprintf("%d", ch.Subscribers)),
				)
			}
			if len(sorted) > 10 {
				fmt.Fprintf(&appContent, "    %s\n", dimStyle.Render(fmt.Sprintf("... and %d more", len(sorted)-10)))
			}
		}

		b.WriteString(appBoxStyle.Render(appContent.String()))
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("  Press q to quit • refreshes every 2s"))
	b.WriteString("\n")

	return b.String()
}

func connStyle(n int) string {
	s := fmt.Sprintf("%d", n)
	if n == 0 {
		return dimStyle.Render(s)
	}
	if n > 1000 {
		return warnStyle.Render(s)
	}
	return goodStyle.Render(s)
}

func fetchStats(addr string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s/stats", addr))
		if err != nil {
			return errMsg(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg(err)
		}

		var stats StatsResponse
		if err := json.Unmarshal(body, &stats); err != nil {
			return errMsg(err)
		}

		return statsMsg(stats)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
