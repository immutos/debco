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

package secondstage

import (
	"fmt"
	"os"
	"path/filepath"

	cp "github.com/otiai10/copy"
)

// usrMergeDirectories is the complete list of directories that can be merged into /usr.
var usrMergeDirectories = []string{"/bin", "/lib", "/lib32", "/lib64", "/libo32", "/libx32", "/sbin"}

// MergeUsr merges the /usr directory into the root filesystem
// See: https://wiki.debian.org/UsrMerge
func MergeUsr() error {
	for _, dir := range usrMergeDirectories {
		// The architecture does not have this directory.
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		// The directory is already usr merged.
		if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			continue
		}

		canonDir := filepath.Join("/usr", dir)
		if err := cp.Copy(dir, canonDir, cp.Options{OnSymlink: func(src string) cp.SymlinkAction {
			return cp.Shallow
		}}); err != nil {
			return fmt.Errorf("failed to copy %s to %s: %w", dir, canonDir, err)
		}

		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("failed to remove %s: %w", dir, err)
		}

		if err := os.Symlink(canonDir, dir); err != nil {
			return fmt.Errorf("failed to symlink %s -> /usr%s: %w", dir, canonDir, err)
		}
	}

	return nil
}
