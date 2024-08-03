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

package buildkit_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/adrg/xdg"
	"github.com/containerd/containerd/platforms"
	"github.com/dpeckett/debco/internal/buildkit"
	"github.com/dpeckett/debco/internal/testutil"
	"github.com/dpeckett/debco/internal/unpack"
	"github.com/dpeckett/debco/internal/util/diskcache"
	"github.com/gregjones/httpcache"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	testutil.SetupGlobals(t)

	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker is not available")
	}

	ctx := context.Background()
	tempDir := t.TempDir()

	binaryDir := filepath.Join(tempDir, "bin")
	require.NoError(t, buildDebcoBinary(ctx, binaryDir))

	certsDir := filepath.Join(tempDir, "certs")
	err := os.MkdirAll(certsDir, 0o700)
	require.NoError(t, err)

	b := buildkit.New("debco-test", certsDir)

	require.NoError(t, b.StartDaemon(ctx))
	t.Cleanup(func() {
		require.NoError(t, b.StopDaemon(ctx))
	})

	packagesDir := filepath.Join(tempDir, "packages")
	require.NoError(t, os.MkdirAll(packagesDir, 0o755))

	require.NoError(t, downloadPackages(packagesDir))

	debs, err := os.ReadDir(packagesDir)
	require.NoError(t, err)

	var packagePaths []string
	for _, e := range debs {
		packagePaths = append(packagePaths, filepath.Join(packagesDir, e.Name()))
	}

	dpkgConfArchivePath, dataArchivePaths, err := unpack.Unpack(ctx, tempDir, packagePaths)
	require.NoError(t, err)

	outputDir := t.TempDir()
	ociArchivePath := filepath.Join(outputDir, "image.tar")

	sourceDateEpoch, err := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	err = b.Build(ctx, buildkit.BuildOptions{
		OCIArchivePath:        ociArchivePath,
		RecipePath:            "testdata/debco.yaml",
		SecondStageBinaryPath: filepath.Join(binaryDir, "debco"),
		SourceDateEpoch:       sourceDateEpoch,
		PlatformOpts: []buildkit.PlatformBuildOptions{
			{
				Platform:            platforms.MustParse("linux/amd64"),
				BuildContextDir:     tempDir,
				DpkgConfArchivePath: dpkgConfArchivePath,
				DataArchivePaths:    dataArchivePaths,
			},
			{
				// Terrible but binfmt_misc will save us.
				Platform:            platforms.MustParse("linux/arm64"),
				BuildContextDir:     tempDir,
				DpkgConfArchivePath: dpkgConfArchivePath,
				DataArchivePaths:    dataArchivePaths,
			},
		},
	})
	require.NoError(t, err)

	// Check that the OCI tarball was created.
	require.FileExists(t, ociArchivePath)
}

// The minimum set of packages required to build a functioning Debian base system.
var packageURLs = []string{
	"https://ftp.debian.org/debian/pool/main/b/base-files/base-files_12.4+deb12u6_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/b/base-passwd/base-passwd_3.6.1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/c/ca-certificates/ca-certificates_20230311_all.deb",
	"https://ftp.debian.org/debian/pool/main/c/coreutils/coreutils_9.1-1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/d/dash/dash_0.5.12-2_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/d/debconf/debconf_1.5.82_all.deb",
	"https://ftp.debian.org/debian/pool/main/d/debianutils/debianutils_5.7-0.5~deb12u1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/d/diffutils/diffutils_3.8-4_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/d/dpkg/dpkg_1.21.22_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/gcc-12/gcc-12-base_12.2.0-14_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/a/acl/libacl1_2.3.1-3_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/a/attr/libattr1_2.5.1-4_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/b/bzip2/libbz2-1.0_1.0.8-5+b1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/glibc/libc-bin_2.36-9+deb12u7_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/glibc/libc6_2.36-9+deb12u7_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/libx/libxcrypt/libcrypt1_4.4.33-2_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/c/cdebconf/libdebconfclient0_0.270_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/gcc-12/libgcc-s1_12.2.0-14_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/gmp/libgmp10_6.2.1+dfsg1-1.1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/gcc-12/libgomp1_12.2.0-14_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/x/xz-utils/liblzma5_5.4.1-0.2_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/libm/libmd/libmd0_1.0.4-2_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/p/pcre2/libpcre2-8-0_10.42-1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/libs/libselinux/libselinux1_3.4-1+b6_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/o/openssl/libssl3_3.0.13-1~deb12u1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/g/gcc-12/libstdc++6_12.2.0-14_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/libz/libzstd/libzstd1_1.5.4+dfsg2-5_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/m/mawk/mawk_1.3.4.20200120-3.1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/n/netbase/netbase_6.4_all.deb",
	"https://ftp.debian.org/debian/pool/main/o/openssl/openssl_3.0.13-1~deb12u1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/p/perl/perl-base_5.36.0-7+deb12u1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/s/sed/sed_4.9-1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/t/tar/tar_1.34+dfsg-1.2+deb12u1_amd64.deb",
	"https://ftp.debian.org/debian/pool/main/t/tzdata/tzdata_2024a-0+deb12u1_all.deb",
	"https://ftp.debian.org/debian/pool/main/z/zlib/zlib1g_1.2.13.dfsg-1_amd64.deb",
}

func downloadPackages(packagesDir string) error {
	cacheDir, err := xdg.CacheFile("debco")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	// Reuse the main debco cache for downloading packages.
	cache, err := diskcache.NewDiskCache(cacheDir, "http")
	if err != nil {
		return err
	}

	httpClient := &http.Client{
		Transport: httpcache.NewTransport(cache),
	}

	for _, pkgURL := range packageURLs {
		url, err := url.Parse(pkgURL)
		if err != nil {
			return err
		}

		slog.Info("Downloading package", slog.String("name", filepath.Base(url.Path)))

		req, err := http.NewRequest(http.MethodGet, url.String(), nil)
		if err != nil {
			return err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			return err
		}

		f, err := os.Create(filepath.Join(packagesDir, filepath.Base(url.Path)))
		if err != nil {
			return err
		}

		if _, err = io.Copy(f, resp.Body); err != nil {
			return err
		}

		if err := f.Close(); err != nil {
			return err
		}

		if err := resp.Body.Close(); err != nil {
			return err
		}
	}

	return nil
}

func buildDebcoBinary(ctx context.Context, binaryDir string) error {
	moduleRootDir := testutil.Root()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(binaryDir, "debco"), moduleRootDir)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build debco binary: %w: %s", err, output)
	}

	return nil
}
