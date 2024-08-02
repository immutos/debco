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

func TestCreateOrUpdateUser(t *testing.T) {
	dir := t.TempDir()

	// Set the file paths to the temp directory.
	groupFilePath = filepath.Join(dir, "group")
	groupShadowFilePath = filepath.Join(dir, "gshadow")
	passwdFilePath = filepath.Join(dir, "passwd")
	shadowFilePath = filepath.Join(dir, "shadow")

	// copy the test files to the temp directory.
	require.NoError(t, cp.Copy("testdata/reference", dir))

	// Create a group for the user.
	require.NoError(t, CreateOrUpdateGroup(Group{Name: "testgroup"}))

	// Create a new user.
	err := CreateOrUpdateUser(User{
		Name:     "testuser",
		Groups:   []string{"testgroup", "sudo"},
		HomeDir:  "/home/testuser",
		Shell:    "/bin/bash",
		Password: "testpassword",
	})
	require.NoError(t, err)

	require.FileExists(t, passwdFilePath)
	require.FileExists(t, passwdFilePath+"-")

	buf, err := os.ReadFile(passwdFilePath)
	require.NoError(t, err)

	expectedPasswdContents, err := os.ReadFile("testdata/user_test/passwd")
	require.NoError(t, err)

	require.Equal(t, string(expectedPasswdContents), string(buf))

	require.FileExists(t, shadowFilePath)
	require.FileExists(t, shadowFilePath+"-")

	buf, err = os.ReadFile(shadowFilePath)
	require.NoError(t, err)

	// Mask out the bcrypt hash.
	start := strings.Index(string(buf), "$2a$10") + 6
	end := strings.Index(string(buf[start:]), ":")

	buf = []byte(string(buf[:start]) + string(buf[start+end:]))

	expectedShadowContents, err := os.ReadFile("testdata/user_test/shadow")
	require.NoError(t, err)

	require.Equal(t, string(expectedShadowContents), string(buf))

	require.FileExists(t, groupFilePath)
	require.FileExists(t, groupFilePath+"-")

	buf, err = os.ReadFile(groupFilePath)
	require.NoError(t, err)

	expectedGroupContents, err := os.ReadFile("testdata/user_test/group")
	require.NoError(t, err)

	require.Equal(t, string(expectedGroupContents), string(buf))
}
