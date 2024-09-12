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

package database_test

import (
	"testing"

	debtypes "github.com/dpeckett/deb822/types"
	"github.com/dpeckett/deb822/types/dependency"
	"github.com/dpeckett/deb822/types/version"
	"github.com/immutos/debco/internal/database"
	"github.com/immutos/debco/internal/testutil"
	"github.com/immutos/debco/internal/types"
	"github.com/stretchr/testify/require"
)

func TestPackageDB(t *testing.T) {
	testutil.SetupGlobals(t)

	db := database.NewPackageDB()

	db.AddAll([]types.Package{
		{
			Package: debtypes.Package{
				Name:    "foo",
				Version: version.MustParse("1.0"),
			},
		},
		{
			Package: debtypes.Package{
				Name:    "foo",
				Version: version.MustParse("1.1"),
			},
		},
		{
			Package: debtypes.Package{
				Name:    "bar",
				Version: version.MustParse("2.0"),
			},
		},
	})

	require.Equal(t, 3, db.Len())

	t.Run("Get", func(t *testing.T) {
		t.Run("All", func(t *testing.T) {
			packages := db.Get("foo")

			require.Len(t, packages, 2)
		})

		t.Run("Strictly Earlier", func(t *testing.T) {
			packages := db.StrictlyEarlier("foo", version.MustParse("1.1"))

			require.Len(t, packages, 1)
			require.Equal(t, "foo", packages[0].Name)
			require.Equal(t, version.MustParse("1.0"), packages[0].Version)
		})

		t.Run("Earlier or Equal", func(t *testing.T) {
			packages := db.EarlierOrEqual("foo", version.MustParse("1.1"))

			require.Len(t, packages, 2)
		})

		t.Run("Exact Version", func(t *testing.T) {
			pkg, exists := db.ExactlyEqual("foo", version.MustParse("1.0"))

			require.True(t, exists)
			require.Equal(t, "foo", pkg.Name)
			require.Equal(t, version.MustParse("1.0"), pkg.Version)
		})

		t.Run("Exact Version (Missing)", func(t *testing.T) {
			_, exists := db.ExactlyEqual("foo", version.MustParse("1.2"))

			require.False(t, exists)
		})

		t.Run("Later or Equal", func(t *testing.T) {
			packages := db.LaterOrEqual("foo", version.MustParse("1.0"))

			require.Len(t, packages, 2)
			require.Equal(t, "foo", packages[0].Name)
			require.Equal(t, version.MustParse("1.0"), packages[0].Version)
			require.Equal(t, version.MustParse("1.1"), packages[1].Version)
		})

		t.Run("Strictly Later", func(t *testing.T) {
			packages := db.StrictlyLater("foo", version.MustParse("1.0"))

			require.Len(t, packages, 1)
			require.Equal(t, "foo", packages[0].Name)
			require.Equal(t, version.MustParse("1.1"), packages[0].Version)
		})
	})

	t.Run("Add and Remove", func(t *testing.T) {
		pkg := types.Package{
			Package: debtypes.Package{
				Name:    "baz",
				Version: version.MustParse("3.0"),
			},
		}

		db.Add(pkg)

		require.Equal(t, 4, db.Len())

		db.Remove(pkg)

		require.Equal(t, 3, db.Len())
	})

	t.Run("Virtual Packages", func(t *testing.T) {
		pkg := types.Package{
			Package: debtypes.Package{
				Name:    "baz",
				Version: version.MustParse("3.0"),
				Provides: dependency.Dependency{
					Relations: []dependency.Relation{
						{
							Possibilities: []dependency.Possibility{{Name: "bazz"}},
						},
					},
				},
			},
		}

		db.Add(pkg)

		packages := db.Get("bazz")

		require.Len(t, packages, 1)
		require.Equal(t, "bazz", packages[0].Name)
		require.True(t, packages[0].IsVirtual)
		require.Equal(t, "baz", packages[0].Providers[0].Name)
		require.Equal(t, version.MustParse("3.0"), packages[0].Providers[0].Version)
	})
}
