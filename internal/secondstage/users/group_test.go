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

package users

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateGroup(t *testing.T) {
	dir := t.TempDir()

	// Set the file paths to the temp directory.
	groupFilePath = filepath.Join(dir, "group")
	groupShadowFilePath = filepath.Join(dir, "gshadow")
	passwdFilePath = filepath.Join(dir, "passwd")
	shadowFilePath = filepath.Join(dir, "shadow")

	// copy the test files to the temp directory.
	require.NoError(t, cp.Copy("testdata/reference", dir))

	// Create a new group.
	require.NoError(t, CreateOrUpdateGroup(Group{
		Name:    "testgroup",
		Members: []string{"user1", "user2"},
	}))

	// Update an existing group.
	require.NoError(t, CreateOrUpdateGroup(Group{
		Name:    "sudo",
		GID:     27,
		Members: []string{"user1", "user2"},
		System:  true,
	}))

	// Test invalid group name.
	require.Error(t, CreateOrUpdateGroup(Group{Name: "test:group"}))
	require.Error(t, CreateOrUpdateGroup(Group{Name: strings.Repeat("a", 33)}))

	// Confirm the group file contents.
	require.FileExists(t, groupFilePath)
	require.FileExists(t, groupFilePath+"-")

	buf, err := os.ReadFile(groupFilePath)
	require.NoError(t, err)

	expectedGroupContents, err := os.ReadFile("testdata/group_test/group")
	require.NoError(t, err)

	require.Equal(t, string(expectedGroupContents), string(buf))

	// Confirm the group shadow file contents.
	require.FileExists(t, groupShadowFilePath)
	require.FileExists(t, groupShadowFilePath+"-")

	buf, err = os.ReadFile(groupShadowFilePath)
	require.NoError(t, err)

	expectedGroupShadowContents, err := os.ReadFile("testdata/group_test/gshadow")
	require.NoError(t, err)

	require.Equal(t, string(expectedGroupShadowContents), string(buf))
}
