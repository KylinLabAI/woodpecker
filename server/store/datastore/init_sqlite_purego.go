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

//go:build !cgo

package datastore

import (
	"database/sql"

	// modernc.org/sqlite is a pure-Go translation of SQLite and therefore
	// does not require a C toolchain. We use it (instead of mattn/go-sqlite3)
	// when building with CGO_ENABLED=0, e.g. cross-compiling the server for
	// Linux from a host that has no Linux C cross-compiler.
	//
	// It is registered under the "sqlite3" driver name so the rest of the
	// code base, which expects the mattn/go-sqlite3 driver name, keeps working
	// unchanged.
	moderncSqlite "modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite3", &moderncSqlite.Driver{})
}
