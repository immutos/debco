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
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/gregjones/httpcache"
	"github.com/immutos/debco/internal/buildkit"
	"github.com/immutos/debco/internal/testutil"
	"github.com/immutos/debco/internal/unpack"
	"github.com/immutos/debco/internal/util/diskcache"
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

	// Make sure we're starting with a clean slate.
	_ = b.StopDaemon(ctx)
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

	dpkgDatabaseArchivePath, dataArchivePaths, err := unpack.Unpack(ctx, tempDir, packagePaths)
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
				Platform:                platforms.MustParse("linux/amd64"),
				BuildContextDir:         tempDir,
				DpkgDatabaseArchivePath: dpkgDatabaseArchivePath,
				DataArchivePaths:        dataArchivePaths,
			},
			{
				// Terrible but binfmt_misc will save us.
				Platform:                platforms.MustParse("linux/arm64"),
				BuildContextDir:         tempDir,
				DpkgDatabaseArchivePath: dpkgDatabaseArchivePath,
				DataArchivePaths:        dataArchivePaths,
			},
		},
	})
	require.NoError(t, err)

	// Check that the OCI tarball was created.
	require.FileExists(t, ociArchivePath)
}

const repositoryURL = "https://snapshot.debian.org/archive/debian/20240801T024036Z"

// The minimum set of packages required to build a functioning Debian base system.
var packages = []struct {
	url    string
	sha256 string
}{
	{
		url:    repositoryURL + "/pool/main/b/base-files/base-files_12.4+deb12u6_amd64.deb",
		sha256: "6de342750e6a3940b321a5d056d0e17512d5ad3eb2fcf1fa6dbd783fcb4b7f0e",
	},
	{
		url:    repositoryURL + "/pool/main/b/base-passwd/base-passwd_3.6.1_amd64.deb",
		sha256: "908ca1b35125f49125ae56945a72bc11ce0fcec85a8d980d10d83bb3a610f518",
	},
	{
		url:    repositoryURL + "/pool/main/c/ca-certificates/ca-certificates_20230311_all.deb",
		sha256: "5308b9bd88eebe2a48be3168cb3d87677aaec5da9c63ad0cf561a29b8219115c",
	},
	{
		url:    repositoryURL + "/pool/main/c/coreutils/coreutils_9.1-1_amd64.deb",
		sha256: "61038f857e346e8500adf53a2a0a20859f4d3a3b51570cc876b153a2d51a3091",
	},
	{
		url:    repositoryURL + "/pool/main/d/dash/dash_0.5.12-2_amd64.deb",
		sha256: "33ea40061da2f1a861ec46212b2b6a34f0776a049b1a3f0abce2fb8cb994258f",
	},
	{
		url:    repositoryURL + "/pool/main/d/debconf/debconf_1.5.82_all.deb",
		sha256: "74ab14194a3762b2fc717917dcfda42929ab98e3c59295a063344dc551cd7cc8",
	},
	{
		url:    repositoryURL + "/pool/main/d/debianutils/debianutils_5.7-0.5~deb12u1_amd64.deb",
		sha256: "55f951359670eb3236c9e2ccd5fac9ccb3db734f5a22aff21589e7a30aee48c9",
	},
	{
		url:    repositoryURL + "/pool/main/d/diffutils/diffutils_3.8-4_amd64.deb",
		sha256: "8bdfedc14c1035e3750e9f055ac9c1ecd9b5d05d9e6dc6466c4e9237eef407dd",
	},
	{
		url:    repositoryURL + "/pool/main/d/dpkg/dpkg_1.21.22_amd64.deb",
		sha256: "9d97f27d8a8a06dd4800e8e0291337ca02e11cdfd7df09a4566a982a6d9fe4c4",
	},
	{
		url:    repositoryURL + "/pool/main/g/gcc-12/gcc-12-base_12.2.0-14_amd64.deb",
		sha256: "1a03df5a57833d65b5bb08cfa19d50e76f29088dc9e64fb934af42d9023a0807",
	},
	{
		url:    repositoryURL + "/pool/main/a/acl/libacl1_2.3.1-3_amd64.deb",
		sha256: "8be9df5795114bfe90e2be3d208ef47a5edd3fc7b3e20d387a597486d444e5e2",
	},
	{
		url:    repositoryURL + "/pool/main/a/attr/libattr1_2.5.1-4_amd64.deb",
		sha256: "c4945123d66d0503ba42e2fc0585abc76d0838978c6d277b9cc37a4da25d1a34",
	},
	{
		url:    repositoryURL + "/pool/main/b/bzip2/libbz2-1.0_1.0.8-5+b1_amd64.deb",
		sha256: "54149da3f44b22d523b26b692033b84503d822cc5122fed606ea69cc83ca5aeb",
	},
	{
		url:    repositoryURL + "/pool/main/g/glibc/libc-bin_2.36-9+deb12u7_amd64.deb",
		sha256: "687687d1ace90565cc451b1be527914246123968b747c823e276cd7f8b57ba3d",
	},
	{
		url:    repositoryURL + "/pool/main/g/glibc/libc6_2.36-9+deb12u7_amd64.deb",
		sha256: "eba944bd99c2f5142baf573e6294a70f00758083bc3c2dca4c9e445943a3f8e6",
	},
	{
		url:    repositoryURL + "/pool/main/libx/libxcrypt/libcrypt1_4.4.33-2_amd64.deb",
		sha256: "f5f60a5cdfd4e4eaa9438ade5078a57741a7a78d659fcb0c701204f523e8bd29",
	},
	{
		url:    repositoryURL + "/pool/main/c/cdebconf/libdebconfclient0_0.270_amd64.deb",
		sha256: "7d2b2b700bae0ba67a13655fabba6a98da3f6ce7dee43d1ee0ac433b7ca1d947",
	},
	{
		url:    repositoryURL + "/pool/main/g/gcc-12/libgcc-s1_12.2.0-14_amd64.deb",
		sha256: "f3d1d48c0599aea85b7f2077a01d285badc42998c1a1e7473935d5cf995c8141",
	},
	{
		url:    repositoryURL + "/pool/main/g/gmp/libgmp10_6.2.1+dfsg1-1.1_amd64.deb",
		sha256: "187aedef2ed763f425c1e523753b9719677633c7eede660401739e9c893482bd",
	},
	{
		url:    repositoryURL + "/pool/main/g/gcc-12/libgomp1_12.2.0-14_amd64.deb",
		sha256: "1dbc499d2055cb128fa4ed678a7adbcced3d882b3509e26d5aa3742a4b9e5b2f",
	},
	{
		url:    repositoryURL + "/pool/main/x/xz-utils/liblzma5_5.4.1-0.2_amd64.deb",
		sha256: "d4b7736e58512a2b047f9cb91b71db5a3cf9d3451192fc6da044c77bf51fe869",
	},
	{
		url:    repositoryURL + "/pool/main/libm/libmd/libmd0_1.0.4-2_amd64.deb",
		sha256: "03539fd30c509e27101d13a56e52eda9062bdf1aefe337c07ab56def25a13eab",
	},
	{
		url:    repositoryURL + "/pool/main/p/pcre2/libpcre2-8-0_10.42-1_amd64.deb",
		sha256: "030db54f4d76cdfe2bf0e8eb5f9efea0233ab3c7aa942d672c7b63b52dbaf935",
	},
	{
		url:    repositoryURL + "/pool/main/libs/libselinux/libselinux1_3.4-1+b6_amd64.deb",
		sha256: "2b07f5287b9105f40158b56e4d70cc1652dac56a408f3507b4ab3d061eed425f",
	},
	{
		url:    repositoryURL + "/pool/main/o/openssl/libssl3_3.0.13-1~deb12u1_amd64.deb",
		sha256: "8e88b98b3fc634721d0899f498d4cf2e62405faaab6582123c7923b1ec8129e1",
	},
	{
		url:    repositoryURL + "/pool/main/g/gcc-12/libstdc++6_12.2.0-14_amd64.deb",
		sha256: "9b1b269020cec6aced3b39f096f7b67edd1f0d4ab24f412cb6506d0800e19cbf",
	},
	{
		url:    repositoryURL + "/pool/main/libz/libzstd/libzstd1_1.5.4+dfsg2-5_amd64.deb",
		sha256: "6315b5ac38b724a710fb96bf1042019398cb656718b1522279a5185ed39318fa",
	},
	{
		url:    repositoryURL + "/pool/main/m/mawk/mawk_1.3.4.20200120-3.1_amd64.deb",
		sha256: "bcbc83f391854ea9d50ce2a4101aacf330de3b8b71d81a798faadba14a157f78",
	},
	{
		url:    repositoryURL + "/pool/main/n/netbase/netbase_6.4_all.deb",
		sha256: "29b23c48c0fe6f878e56c5ddc9f65d1c05d729360f3690a593a8c795031cd867",
	},
	{
		url:    repositoryURL + "/pool/main/o/openssl/openssl_3.0.13-1~deb12u1_amd64.deb",
		sha256: "262faebdc38b64e9e0553388e8608b0b6ae1b56871e7a8b09737ab0f2df11f8c",
	},
	{
		url:    repositoryURL + "/pool/main/p/perl/perl-base_5.36.0-7+deb12u1_amd64.deb",
		sha256: "b4327c2d8e2ca92402205ac6b5845b3110fa2a1d50925c0e61c39624583a8baf",
	},
	{
		url:    repositoryURL + "/pool/main/s/sed/sed_4.9-1_amd64.deb",
		sha256: "177cacdfe9508448d84bf25534a87a7fcc058d8e2dcd422672851ea13f2115df",
	},
	{
		url:    repositoryURL + "/pool/main/t/tar/tar_1.34+dfsg-1.2+deb12u1_amd64.deb",
		sha256: "24fb92e98c2969171f81a8b589263d705f6b1670f95d121cd74c810d4605acc3",
	},
	{
		url:    repositoryURL + "/pool/main/t/tzdata/tzdata_2024a-0+deb12u1_all.deb",
		sha256: "0ca0baec1fca55df56039047a631fc1541c5a44c1c4879d553aaa3a70844eb12",
	},
	{
		url:    repositoryURL + "/pool/main/z/zlib/zlib1g_1.2.13.dfsg-1_amd64.deb",
		sha256: "d7dd1d1411fedf27f5e27650a6eff20ef294077b568f4c8c5e51466dc7c08ce4",
	},
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

	for _, pkg := range packages {
		url, err := url.Parse(pkg.url)
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
			_ = resp.Body.Close()
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		f, err := os.Create(filepath.Join(packagesDir, filepath.Base(url.Path)))
		if err != nil {
			_ = resp.Body.Close()
			return err
		}

		h := sha256.New()
		_, err = io.Copy(f, io.TeeReader(resp.Body, h))
		_ = resp.Body.Close()
		if err != nil {
			_ = f.Close()
			return err
		}

		if hex.EncodeToString(h.Sum(nil)) != pkg.sha256 {
			_ = f.Close()
			return fmt.Errorf("sha256 sum mismatch")
		}

		if err := f.Close(); err != nil {
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
