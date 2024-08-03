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

package diskcache_test

import (
	"testing"

	"github.com/dpeckett/debco/internal/testutil"
	"github.com/dpeckett/debco/internal/util/diskcache"
	"github.com/stretchr/testify/require"
)

func TestDiskCache(t *testing.T) {
	testutil.SetupGlobals(t)

	cacheDir := t.TempDir()

	cache, err := diskcache.NewDiskCache(cacheDir, "test")
	require.NoError(t, err)

	t.Run("Exist", func(t *testing.T) {
		cache.Set("exist", []byte("data"))

		data, ok := cache.Get("exist")
		require.True(t, ok)
		require.Equal(t, []byte("data"), data)
	})

	t.Run("Non Exist", func(t *testing.T) {
		_, ok := cache.Get("non-exist")
		require.False(t, ok)
	})
}
