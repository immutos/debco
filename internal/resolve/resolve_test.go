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

package resolve_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpeckett/deb822"
	"github.com/dpeckett/debco/internal/database"
	"github.com/dpeckett/debco/internal/resolve"
	"github.com/dpeckett/debco/internal/testutil"
	"github.com/dpeckett/debco/internal/types"
	"github.com/dpeckett/uncompr"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	testutil.SetupGlobals(t)

	f, err := os.Open(filepath.Join(testutil.Root(), "testdata/Packages.gz"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})

	dr, err := uncompr.NewReader(f)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, dr.Close())
	})

	decoder, err := deb822.NewDecoder(dr, nil)
	require.NoError(t, err)

	var packageList []types.Package
	require.NoError(t, decoder.Decode(&packageList))

	packageDB := database.NewPackageDB()
	packageDB.AddAll(packageList)

	selectedDB, err := resolve.Resolve(packageDB, []string{"bash=5.2.15-2+b2"}, nil)
	require.NoError(t, err)

	var selectedNameVersions []string
	_ = selectedDB.ForEach(func(pkg types.Package) error {
		selectedNameVersions = append(selectedNameVersions,
			fmt.Sprintf("%s=%s", pkg.Name, pkg.Version))

		return nil
	})

	expectedNameVersions := []string{
		"base-files=12.4+deb12u5",
		"bash=5.2.15-2+b2",
		"debianutils=5.7-0.5~deb12u1",
		"gcc-12-base=12.2.0-14",
		"libc6=2.36-9+deb12u4",
		"libgcc-s1=12.2.0-14",
		"libtinfo6=6.4-4",
		"mawk=1.3.4.20200120-3.1",
	}

	require.ElementsMatch(t, expectedNameVersions, selectedNameVersions)
}
