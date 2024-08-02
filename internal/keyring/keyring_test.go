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

package keyring_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dpeckett/debco/internal/keyring"
	"github.com/dpeckett/debco/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestKeyringRead(t *testing.T) {
	testutil.SetupGlobals(t)

	ctx := context.Background()

	t.Run("Web", func(t *testing.T) {
		keyring, err := keyring.Load(ctx, "https://ftp-master.debian.org/keys/archive-key-12.asc")
		require.NoError(t, err)

		require.NotEmpty(t, keyring)
	})

	t.Run("File", func(t *testing.T) {
		keyring, err := keyring.Load(ctx, filepath.Join(testutil.Root(), "testdata/archive-key-12.asc"))
		require.NoError(t, err)

		require.NotEmpty(t, keyring)
	})
}
