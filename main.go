package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Structs for API Responses ---

type WeatherResponse struct {
	Latitude     float64      `json:"latitude"`
	Longitude    float64      `json:"longitude"`
	CurrentUnits CurrentUnits `json:"current_units"`
	Current      CurrentData  `json:"current"`
}

type CurrentUnits struct {
	Temperature2m string `json:"temperature_2m"`
	WeatherCode   string `json:"weather_code"`
}

type CurrentData struct {
	Time          string  `json:"time"`
	Temperature2m float64 `json:"temperature_2m"`
	WeatherCode   int     `json:"weather_code"`
}

type GeoInfo struct {
	Status  string  `json:"status"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	ISP     string  `json:"isp"`
	Query   string  `json:"query"`
}

// --- Bubble Tea Messages & Model ---

type cacheCheckResultMsg struct {
	hasCache    bool
	geoInfo     *GeoInfo
	weatherData *WeatherResponse
}
type ipFetchedMsg string
type geoInfoFetchedMsg *GeoInfo
type weatherFetchedMsg *WeatherResponse
type errorMsg struct{ err error }

type model struct {
	steps        []string
	index        int
	spinner      spinner.Model
	progress     progress.Model
	width        int
	height       int
	done         bool
	err          error
	geoInfo      *GeoInfo
	weatherData  *WeatherResponse
	forceRefresh bool
}

// --- Bubble Tea Styling ---

var (
	currentStepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("211"))
	doneStyle        = lipgloss.NewStyle().Margin(1, 2)
	checkMark        = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	errorMark        = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).SetString("✗")
)

// --- Helper Functions ---

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getTempColor(temp float64) lipgloss.Style {
	switch {
	case temp <= 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF"))
	case temp > 0 && temp <= 10:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6495ED"))
	case temp > 10 && temp <= 20:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#90EE90"))
	case temp > 20 && temp <= 28:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700"))
	case temp > 28:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500"))
	default:
		return lipgloss.NewStyle()
	}
}

// --- Bubble Tea App ---

func newModel(forceRefresh bool) model {
	steps := []string{
		"Checking local cache...",
		"Fetching public IP...",
		"Fetching geolocation data...",
		"Fetching weather forecast...",
	}

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	return model{
		steps:        steps,
		spinner:      s,
		progress:     p,
		forceRefresh: forceRefresh,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(checkCacheCmd(m.forceRefresh), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}

	case cacheCheckResultMsg:
		if msg.hasCache {
			m.done = true
			m.geoInfo = msg.geoInfo
			m.weatherData = msg.weatherData

			// Give Bubble Tea a moment to render the Printf before quitting.
			quitCmd := tea.Tick(time.Millisecond*80, func(t time.Time) tea.Msg {
				return tea.Quit()
			})

			return m, tea.Batch(
				tea.Printf("%s Found valid data in cache", checkMark),
				quitCmd,
			)
		}
		// Cache miss, proceed to the next step
		m.index++
		progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.steps)))
		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s Cache not found or invalid", checkMark),
			fetchPublicIPCmd(),
		)

	case ipFetchedMsg:
		m.index++
		progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.steps)))
		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s Public IP fetched", checkMark),
			fetchGeoInfoCmd(string(msg)),
		)

	case geoInfoFetchedMsg:
		m.index++
		m.geoInfo = msg
		progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.steps)))
		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s Geolocation for %s fetched and cached", checkMark, msg.City),
			fetchWeatherInfoCmd(msg),
		)

	case weatherFetchedMsg:
		m.index++
		m.weatherData = msg
		m.done = true
		progressCmd := m.progress.SetPercent(1.0)

		// Give Bubble Tea a moment to render the final Printf before quitting.
		quitCmd := tea.Tick(time.Millisecond*80, func(t time.Time) tea.Msg {
			return tea.Quit()
		})

		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s Weather forecast fetched and cached", checkMark),
			quitCmd,
		)

	case errorMsg:
		m.err = msg.err
		return m, tea.Quit

	case progress.FrameMsg:
		newModel, cmd := m.progress.Update(msg)
		if newModel, ok := newModel.(progress.Model); ok {
			m.progress = newModel
		}
		return m, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return doneStyle.Render(fmt.Sprintf("%s Error: %v\n", errorMark, m.err))
	}

	if m.done {
		cityStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141")) // Light purple
		tempStyle := getTempColor(m.weatherData.Current.Temperature2m)

		city := cityStyle.Render(m.geoInfo.City)
		temp := tempStyle.Render(fmt.Sprintf("%.1f°C", m.weatherData.Current.Temperature2m))

		weatherResult := fmt.Sprintf("\n%s — %s\n", city, temp)
		return doneStyle.Render(weatherResult)
	}

	n := len(m.steps)
	w := lipgloss.Width(fmt.Sprintf("%d", n))
	pkgCount := fmt.Sprintf(" %*d/%*d", w, m.index+1, w, n)
	spin := m.spinner.View() + " "
	prog := m.progress.View()
	cellsAvail := max(0, m.width-lipgloss.Width(spin+prog+pkgCount))
	stepName := currentStepStyle.Render(m.steps[m.index])
	info := lipgloss.NewStyle().MaxWidth(cellsAvail).Render(stepName)
	cellsRemaining := max(0, m.width-lipgloss.Width(spin+info+prog+pkgCount))
	gap := strings.Repeat(" ", cellsRemaining)

	return spin + info + gap + prog + pkgCount
}

// --- Commands and Logic ---

func checkCacheCmd(forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		geoCacheFile := filepath.Join(os.TempDir(), "geoinfo_cache.json")
		weatherCacheFile := filepath.Join(os.TempDir(), "weather_cache.json")

		if forceRefresh {
			os.Remove(geoCacheFile)
			os.Remove(weatherCacheFile)
			return cacheCheckResultMsg{hasCache: false}
		}

		// Check for GeoInfo cache
		geoData, err := os.ReadFile(geoCacheFile)
		if err != nil {
			return cacheCheckResultMsg{hasCache: false}
		}
		var geoInfo GeoInfo
		if json.Unmarshal(geoData, &geoInfo) != nil || geoInfo.Status != "success" {
			return cacheCheckResultMsg{hasCache: false}
		}

		// Check for Weather cache
		weatherData, err := os.ReadFile(weatherCacheFile)
		if err != nil {
			return cacheCheckResultMsg{hasCache: false}
		}
		var weatherResp WeatherResponse
		if json.Unmarshal(weatherData, &weatherResp) != nil {
			return cacheCheckResultMsg{hasCache: false}
		}

		// If both caches are valid
		return cacheCheckResultMsg{
			hasCache:    true,
			geoInfo:     &geoInfo,
			weatherData: &weatherResp,
		}
	}
}

func fetchPublicIPCmd() tea.Cmd {
	return func() tea.Msg {
		ip, err := getPublicIP()
		if err != nil {
			return errorMsg{err}
		}
		return ipFetchedMsg(ip)
	}
}

func fetchGeoInfoCmd(ip string) tea.Cmd {
	return func() tea.Msg {
		geoInfo, err := getGeoInfo(ip)
		if err != nil {
			return errorMsg{err}
		}
		return geoInfoFetchedMsg(geoInfo)
	}
}

func fetchWeatherInfoCmd(geoInfo *GeoInfo) tea.Cmd {
	return func() tea.Msg {
		weatherData, err := getWeatherInfo(geoInfo)
		if err != nil {
			return errorMsg{err}
		}
		return weatherFetchedMsg(weatherData)
	}
}

func getPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func getGeoInfo(ip string) (*GeoInfo, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s", ip)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	httpBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading API response body: %w", err)
	}
	cacheFile := filepath.Join(os.TempDir(), "geoinfo_cache.json")
	if err := os.WriteFile(cacheFile, httpBody, 0644); err != nil {
		log.Printf("warning: failed to write geolocation cache: %v", err)
	}
	var geoInfo GeoInfo
	if err := json.Unmarshal(httpBody, &geoInfo); err != nil {
		return nil, err
	}
	if geoInfo.Status != "success" {
		return nil, fmt.Errorf("geolocation API failed with status: %s", geoInfo.Status)
	}
	return &geoInfo, nil
}

func getWeatherInfo(geoInfo *GeoInfo) (*WeatherResponse, error) {
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,weather_code", geoInfo.Lat, geoInfo.Lon)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned an unexpected status: %s", resp.Status)
	}
	httpBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading API response body: %w", err)
	}
	cacheFile := filepath.Join(os.TempDir(), "weather_cache.json")
	if err := os.WriteFile(cacheFile, httpBody, 0644); err != nil {
		log.Printf("warning: failed to write weather cache: %v", err)
	}
	var weatherData WeatherResponse
	if err := json.Unmarshal(httpBody, &weatherData); err != nil {
		return nil, fmt.Errorf("error decoding weather data: %w", err)
	}
	return &weatherData, nil
}

// --- Main Function ---

func main() {
	clearCache := flag.Bool("clear", false, "Force fetch new data by clearing the cache")
	flag.Parse()

	if *clearCache {
		// Create a faint style for the message
		faintStyle := lipgloss.NewStyle().Faint(true)
		// Render the message with the style and print it
		fmt.Println(faintStyle.Render("Cache will be cleared on this run."))
	}

	if _, err := tea.NewProgram(newModel(*clearCache)).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
