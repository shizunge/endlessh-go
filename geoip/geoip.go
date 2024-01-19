// Copyright (C) 2021-2024 Shizun Ge
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

package geoip

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

type GeoOption struct {
	GeoipSupplier     string
	MaxMindDbFileName string
}

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

func geohashAndLocationFromIpapi(ipAddr string) (string, string, string, error) {
	var geo ipapi
	response, err := http.Get("http://ip-api.com/json/" + ipAddr)
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
		return "s000", "Unknown", "Unknown", fmt.Errorf("failed to query %v via ip-api: status: %v, message: %v", ipAddr, geo.Status, geo.Message)
	}

	gh := geohash.EncodeAuto(geo.Latitude, geo.Longitude)
	country := composeCountry(geo.CountryName)
	location := composeLocation(geo.CountryName, geo.RegionName, geo.City)

	return gh, country, location, nil
}

func geohashAndLocationFromMaxMindDb(ipAddr, maxMindDbFileName string) (string, string, string, error) {
	db, err := geoip2.Open(maxMindDbFileName)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}
	defer db.Close()
	// If you are using strings that may be invalid, check that ip is not nil
	ip := net.ParseIP(ipAddr)
	cityRecord, err := db.City(ip)
	if err != nil {
		return "s000", "Unknown", "Unknown", err
	}
	countryName := cityRecord.Country.Names["en"]
	cityName := cityRecord.City.Names["en"]
	latitude := cityRecord.Location.Latitude
	longitude := cityRecord.Location.Longitude
	iso := cityRecord.Country.IsoCode
	if latitude == 0 && longitude == 0 {
		// In case of using Country DB, city is not available.
		loc, ok := countryToLocation[iso]
		if ok {
			latitude = loc.Latitude
			longitude = loc.Longitude
		} else {
			if iso != "" {
				// For debugging, adding the iso to the country name.
				countryName = countryName + " (" + iso + ")"
			}
		}
	}
	gh := geohash.EncodeAuto(latitude, longitude)
	country := composeCountry(countryName)
	location := composeLocation(countryName, "", cityName)

	return gh, country, location, nil
}

func GeohashAndLocation(ipAddr string, option GeoOption) (string, string, string, error) {
	switch option.GeoipSupplier {
	case "off":
		return "s000", "Geohash off", "Geohash off", nil
	case "ip-api":
		return geohashAndLocationFromIpapi(ipAddr)
	case "max-mind-db":
		return geohashAndLocationFromMaxMindDb(ipAddr, option.MaxMindDbFileName)
	default:
		return "s000", "Unknown", "Unknown", fmt.Errorf("unknown geoipSupplier %v.", option.GeoipSupplier)
	}
}
