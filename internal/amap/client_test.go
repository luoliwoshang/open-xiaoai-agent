package amap

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestLiveWeather(t *testing.T) {
	t.Parallel()

	client := NewClient("amap-key")
	client.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://restapi.amap.com/v3/weather/weatherInfo?city=310000&extensions=base&key=amap-key&output=JSON" {
				t.Fatalf("request url = %q", req.URL.String())
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"status":"1",
					"info":"OK",
					"infocode":"10000",
					"lives":[
						{
							"province":"上海",
							"city":"上海市",
							"adcode":"310000",
							"weather":"晴",
							"temperature":"26",
							"winddirection":"东南",
							"windpower":"3",
							"humidity":"61",
							"reporttime":"2026-04-24 18:00:00"
						}
					]
				}`)),
				Header: make(http.Header),
			}, nil
		}),
	})

	live, err := client.LiveWeather(context.Background(), "310000")
	if err != nil {
		t.Fatalf("LiveWeather() error = %v", err)
	}
	if live.City != "上海市" {
		t.Fatalf("live.City = %q", live.City)
	}
	if live.Weather != "晴" {
		t.Fatalf("live.Weather = %q", live.Weather)
	}
	if live.Temperature != "26" {
		t.Fatalf("live.Temperature = %q", live.Temperature)
	}
	if live.ReportTime != "2026-04-24 18:00:00" {
		t.Fatalf("live.ReportTime = %q", live.ReportTime)
	}
}

func TestLiveWeather_RequiresAPIKey(t *testing.T) {
	t.Parallel()

	client := NewClient("")
	if _, err := client.LiveWeather(context.Background(), "310000"); err == nil {
		t.Fatal("LiveWeather() error = nil, want non-nil")
	}
}
