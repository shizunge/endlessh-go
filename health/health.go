// Copyright (C) 2026 Paolo Asperti
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

package health

import (
	"net"
	"os"
	"time"

	"github.com/golang/glog"
)

const (
	DefaultHost    = "127.0.0.1"
	DefaultPort    = "51000"
	DefaultTimeout = 3 * time.Second
)

func StartListener(host, port string) {
	addr := host + ":" + port
	go func() {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			glog.Errorf("Error listening for healthcheck on %v: %v", addr, err)
			os.Exit(1)
		}
		defer l.Close()
		for {
			conn, err := l.Accept()
			if err != nil {
				glog.Errorf("Error accepting healthcheck connection: %v", err)
				os.Exit(1)
			}
			conn.Close()
		}
	}()
}

func Probe(host, port string) bool {
	addr := host + ":" + port
	timeout := DefaultTimeout
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
