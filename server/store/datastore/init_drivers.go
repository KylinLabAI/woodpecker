// Copyright 2018 Drone.IO Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datastore

// Supported database drivers.
//
// Defined once, build-tag free, so the cgo and pure-Go sqlite registration
// files (init_cgo.go / init_sqlite_purego.go) cannot silently diverge.
const (
	DriverSqlite   = "sqlite3"
	DriverMysql    = "mysql"
	DriverPostgres = "postgres"
)

func SupportedDriver(driver string) bool {
	switch driver {
	case DriverMysql, DriverPostgres, DriverSqlite:
		return true
	default:
		return false
	}
}
