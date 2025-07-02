// package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type WeatherResponse struct {
	Latitude             float64      `json:"latitude"`
	Longitude            float64      `json:"longitude"`
	GenerationTimeMs     float64      `json:"generationtime_ms"`
	UtcOffsetSeconds     int          `json:"utc_offset_seconds"`
	Timezone             string       `json:"timezone"`
	TimezoneAbbreviation string       `json:"timezone_abbreviation"`
	Elevation            float64      `json:"elevation"`
	CurrentUnits         CurrentUnits `json:"current_units"`
	Current              CurrentData  `json:"current"`
}

type CurrentUnits struct {
	Time          string `json:"time"`
	Interval      string `json:"interval"`
	Temperature2m string `json:"temperature_2m"`
	WeatherCode   string `json:"weather_code"`
}

type CurrentData struct {
	Time          string  `json:"time"`
	Interval      int     `json:"interval"`
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
	cacheFile := filepath.Join(os.TempDir(), "geoinfo_cache.json")

	fileInfo, err := os.Stat(cacheFile)

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error checking cache file stats: %w", err)
	}

	if err == nil {
		age := time.Since(fileInfo.ModTime())

		cacheTTL := 60 * time.Minute

		if age < cacheTTL {
			cachedData, readErr := os.ReadFile(cacheFile)
			if readErr == nil {
				var geoInfo GeoInfo
				if json.Unmarshal(cachedData, &geoInfo) == nil && geoInfo.Status == "success" {
					return &geoInfo, nil
				}
			}
		}
	}

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

	var weatherData WeatherResponse
	err = json.NewDecoder(resp.Body).Decode(&weatherData)
	if err != nil {
		return nil, fmt.Errorf("error decoding weather data: %w", err)
	}

	return &weatherData, nil
}

func main() {
	ip, err := getPublicIP()
	if err != nil {
		log.Fatalf("error fetching public IP: %v", err)
	}

	geoInfo, err := getGeoInfo(ip)
	if err != nil {
		log.Fatalf("error fetching geolocation: %v", err)
	}

	fmt.Println("---")
	fmt.Printf("Location Found: %s, %s\n", geoInfo.City, geoInfo.Country)
	fmt.Printf("Internet Provider: %s\n", geoInfo.ISP)
	fmt.Printf("Coordinates: Latitude=%f, Longitude=%f\n", geoInfo.Lat, geoInfo.Lon)
	fmt.Println("---")

	weatherData, err := getWeatherInfo(geoInfo)
	if err != nil {
		log.Fatalf("error fetching weather data: %v", err)
	}

	fmt.Printf("\n%s\n", geoInfo.City)
	fmt.Printf("%.1f°C\n", weatherData.Current.Temperature2m)
}
