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

package hashreader_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/dpeckett/debco/internal/testutil"
	"github.com/dpeckett/debco/internal/util/hashreader"
	"github.com/stretchr/testify/require"
)

func TestHashReader(t *testing.T) {
	testutil.SetupGlobals(t)

	data := []byte("The quick brown fox jumps over the lazy dog")

	// Create a HashReader
	reader := bytes.NewReader(data)
	hashReader := hashreader.NewReader(reader)

	// Read the data
	readData, err := io.ReadAll(hashReader)
	require.NoError(t, err)
	require.Equal(t, data, readData)

	// Verify the checksum
	expected := "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592"
	require.NoError(t, hashReader.Verify(expected))
}
