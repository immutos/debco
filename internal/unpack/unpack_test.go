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

package unpack_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpeckett/archivefs/tarfs"
	"github.com/dpeckett/debco/internal/testutil"
	"github.com/dpeckett/debco/internal/unpack"
	"github.com/stretchr/testify/require"
)

func TestUnpack(t *testing.T) {
	testutil.SetupGlobals(t)

	tempDir := t.TempDir()

	ctx := context.Background()

	packagePaths := []string{
		filepath.Join(testutil.Root(), "testdata/debs/base-files_12.4+deb12u5_amd64.deb"),
		filepath.Join(testutil.Root(), "testdata/debs/base-passwd_3.6.1_amd64.deb"),
	}

	dpkgConfArchivePath, dataArchivePaths, err := unpack.Unpack(ctx, tempDir, packagePaths)
	require.NoError(t, err)

	require.Len(t, dataArchivePaths, 2)
	require.Equal(t, "base-files_12.4+deb12u5_amd64_data.tar", filepath.Base(dataArchivePaths[0]))
	require.Equal(t, "base-passwd_3.6.1_amd64_data.tar", filepath.Base(dataArchivePaths[1]))

	dpkgConfArchiveFile, err := os.Open(dpkgConfArchivePath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, dpkgConfArchiveFile.Close())
	})

	tarFS, err := tarfs.Open(dpkgConfArchiveFile)
	require.NoError(t, err)

	var filesList []string
	err = fs.WalkDir(tarFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}

		filesList = append(filesList, path)
		return nil
	})
	require.NoError(t, err)

	expectedFilesList := []string{
		"var",
		"var/lib",
		"var/lib/dpkg",
		"var/lib/dpkg/info",
		"var/lib/dpkg/info/base-files.conffiles",
		"var/lib/dpkg/info/base-files.list",
		"var/lib/dpkg/info/base-files.md5sums",
		"var/lib/dpkg/info/base-files.postinst",
		"var/lib/dpkg/info/base-passwd.list",
		"var/lib/dpkg/info/base-passwd.md5sums",
		"var/lib/dpkg/info/base-passwd.postinst",
		"var/lib/dpkg/info/base-passwd.postrm",
		"var/lib/dpkg/info/base-passwd.preinst",
		"var/lib/dpkg/info/base-passwd.templates",
		"var/lib/dpkg/status",
	}

	require.ElementsMatch(t, expectedFilesList, filesList)
}
