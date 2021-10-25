package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pierrre/geohash"
)

type freegeoip struct {
	Ip          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	RegionCode  string  `json:"region_code"`
	RegionName  string  `json:"region_name"`
	City        string  `json:"city"`
	Zipcode     string  `json:"zipcode"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	MetroCode   int     `json:"metro_code"`
	AreaCode    int     `json:"area_code"`
}

func geohashAndLocationFromFreegeoip(address string) (string, string, string, error) {
	var geo freegeoip
	response, err := http.Get("https://freegeoip.live/json/" + address)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}

	err = json.Unmarshal(body, &geo)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}

	var locations []string
	for _, s := range []string{geo.CountryName, geo.RegionName, geo.City} {
		if strings.TrimSpace(s) != "" {
			locations = append(locations, s)
		}
	}
	location := strings.Join(locations, ", ")
	if location == "" {
		location = "Unknown"
	}
	country := geo.CountryName
	if country == "" {
		country = "Unknown"
	}
	gh := geohash.EncodeAuto(geo.Latitude, geo.Longitude)

	return gh, country, location, nil
}

type ipapi struct {
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Ip          string  `json:"query"`
	CountryCode string  `json:"countryCode"`
	CountryName string  `json:"country"`
	RegionCode  string  `json:"region"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	Zipcode     string  `json:"zip"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lon"`
}

func geohashAndLocationFromIpapi(address string) (string, string, string, error) {
	var geo ipapi
	response, err := http.Get("http://ip-api.com/json/" + address)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}

	err = json.Unmarshal(body, &geo)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}

	if geo.Status != "success" {
		return "s000", "Unknown", "Unknown", fmt.Errorf("failed to query %v via ip-api: status: %v, message: %v", address, geo.Status, geo.Message)
	}

	var locations []string
	for _, s := range []string{geo.CountryName, geo.RegionName, geo.City} {
		if strings.TrimSpace(s) != "" {
			locations = append(locations, s)
		}
	}
	location := strings.Join(locations, ", ")
	if location == "" {
		location = "Unknown"
	}
	country := geo.CountryName
	if country == "" {
		country = "Unknown"
	}
	gh := geohash.EncodeAuto(geo.Latitude, geo.Longitude)

	return gh, country, location, nil
}

func geohashAndLocation(address string, geoipSupplier string) (string, string, string, error) {
	switch geoipSupplier {
	case "off":
		return "s000", "Geohash off", "Geohash off", nil
	case "ip-api":
		return geohashAndLocationFromIpapi(address)
	case "freegeoip":
		return geohashAndLocationFromFreegeoip(address)
	default:
		return "s000", "Unknown", "Unknown", fmt.Errorf("unknown geoipSupplier %v.", geoipSupplier)
	}
}
