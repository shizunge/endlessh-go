// Copyright (C) 2021 Shizun Ge
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/oschwald/geoip2-golang"
	"github.com/pierrre/geohash"
)

var (
	maxMindDbFileName *string
)

func composeLocation(country string, region string, city string) string {
	var locations []string
	for _, s := range []string{country, region, city} {
		if strings.TrimSpace(s) != "" {
			locations = append(locations, s)
		}
	}
	location := strings.Join(locations, ", ")
	if location == "" {
		return "Unknown"
	}
	return location
}

func composeCountry(country string) string {
	if country == "" {
		return "Unknown"
	}
	return country
}

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

	gh := geohash.EncodeAuto(geo.Latitude, geo.Longitude)
	country := composeCountry(geo.CountryName)
	location := composeLocation(geo.CountryName, geo.RegionName, geo.City)

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

	gh := geohash.EncodeAuto(geo.Latitude, geo.Longitude)
	country := composeCountry(geo.CountryName)
	location := composeLocation(geo.CountryName, geo.RegionName, geo.City)

	return gh, country, location, nil
}

func geohashAndLocationFromMaxMindDb(address string) (string, string, string, error) {
	db, err := geoip2.Open(*maxMindDbFileName)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}
	defer db.Close()
	// If you are using strings that may be invalid, check that ip is not nil
	ip := net.ParseIP(address)
	record, err := db.City(ip)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}
	gh := geohash.EncodeAuto(record.Location.Latitude, record.Location.Longitude)
	country := composeCountry(record.Country.Names["en"])
	location := composeLocation(record.Country.Names["en"], "", record.City.Names["en"])

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
	case "max-mind-db":
		return geohashAndLocationFromMaxMindDb(address)
	default:
		return "s000", "Unknown", "Unknown", fmt.Errorf("unknown geoipSupplier %v.", geoipSupplier)
	}
}
