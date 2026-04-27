package amap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const weatherURL = "https://restapi.amap.com/v3/weather/weatherInfo"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type LiveWeather struct {
	Province      string
	City          string
	Adcode        string
	Weather       string
	Temperature   string
	WindDirection string
	WindPower     string
	Humidity      string
	ReportTime    string
}

type weatherResponse struct {
	Status   string `json:"status"`
	Info     string `json:"info"`
	InfoCode string `json:"infocode"`
	Lives    []struct {
		Province      string `json:"province"`
		City          string `json:"city"`
		Adcode        string `json:"adcode"`
		Weather       string `json:"weather"`
		Temperature   string `json:"temperature"`
		WindDirection string `json:"winddirection"`
		WindPower     string `json:"windpower"`
		Humidity      string `json:"humidity"`
		ReportTime    string `json:"reporttime"`
	} `json:"lives"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{},
	}
}

func (c *Client) SetHTTPClient(httpClient *http.Client) {
	if httpClient != nil {
		c.httpClient = httpClient
	}
}

func (c *Client) APIKeyConfigured() bool {
	return c.apiKey != ""
}

func (c *Client) LiveWeather(ctx context.Context, city string) (LiveWeather, error) {
	if !c.APIKeyConfigured() {
		return LiveWeather{}, fmt.Errorf("amap api key is not configured")
	}
	city = strings.TrimSpace(city)
	if city == "" {
		return LiveWeather{}, fmt.Errorf("city is required")
	}

	requestURL, err := url.Parse(weatherURL)
	if err != nil {
		return LiveWeather{}, fmt.Errorf("parse weather url: %w", err)
	}
	query := requestURL.Query()
	query.Set("key", c.apiKey)
	query.Set("city", city)
	query.Set("extensions", "base")
	query.Set("output", "JSON")
	requestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return LiveWeather{}, fmt.Errorf("create weather request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return LiveWeather{}, fmt.Errorf("do weather request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LiveWeather{}, fmt.Errorf("read weather response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return LiveWeather{}, fmt.Errorf("weather request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result weatherResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return LiveWeather{}, fmt.Errorf("decode weather response: %w", err)
	}
	if result.Status != "1" || result.InfoCode != "10000" {
		return LiveWeather{}, fmt.Errorf("weather request failed: info=%s infocode=%s", strings.TrimSpace(result.Info), strings.TrimSpace(result.InfoCode))
	}
	if len(result.Lives) == 0 {
		return LiveWeather{}, fmt.Errorf("weather response has no live data")
	}

	live := result.Lives[0]
	return LiveWeather{
		Province:      strings.TrimSpace(live.Province),
		City:          strings.TrimSpace(live.City),
		Adcode:        strings.TrimSpace(live.Adcode),
		Weather:       strings.TrimSpace(live.Weather),
		Temperature:   strings.TrimSpace(live.Temperature),
		WindDirection: strings.TrimSpace(live.WindDirection),
		WindPower:     strings.TrimSpace(live.WindPower),
		Humidity:      strings.TrimSpace(live.Humidity),
		ReportTime:    strings.TrimSpace(live.ReportTime),
	}, nil
}
