// SPDX-License-Identifier: AGPL-3.0-or-later
/*
 * Copyright (C) 2024 Damian Peckett <damian@pecke.tt>.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package testutil

import (
	"os"
	"path/filepath"
)

// Root finds the root directory of the Go module by looking for the go.mod file.
func Root() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// Look for go.mod file by walking up the directory structure.
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		// Move to the parent directory.
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			panic("could not find root directory")
		}
		dir = parentDir
	}
}
