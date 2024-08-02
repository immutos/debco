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

package unpack

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dpeckett/archivefs/arfs"
	"github.com/dpeckett/archivefs/tarfs"
	"github.com/dpeckett/compressmagic"
	"github.com/dpeckett/deb822"
	"github.com/dpeckett/deb822/types"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
)

func Unpack(ctx context.Context, tempDir string, packagePaths []string) (string, []string, error) {
	var progress *mpb.Progress
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		progress = mpb.NewWithContext(ctx)
		defer progress.Shutdown()
	}

	// Decompress the packages in parallel.
	controlArchivePaths := make([]string, len(packagePaths))
	dataArchivePaths := make([]string, len(packagePaths))
	{
		var bar *mpb.Bar
		if progress != nil {
			bar = progress.AddBar(int64(len(packagePaths)),
				mpb.PrependDecorators(
					decor.Name("Decompressing: "),
					decor.CountersNoUnit("%d / %d"),
				),
				mpb.AppendDecorators(
					decor.Percentage(),
				),
			)
		}

		var g errgroup.Group
		g.SetLimit(runtime.NumCPU())

		for i, packagePath := range packagePaths {
			i := i
			packagePath := packagePath

			g.Go(func() error {
				defer func() {
					if bar != nil {
						bar.Increment()
					}
				}()

				controlArchivePath, dataArchivePath, err := decompressPackage(tempDir, packagePath)
				if err != nil {
					return fmt.Errorf("failed to decompress package %s: %w", filepath.Base(packagePath), err)
				}

				controlArchivePaths[i] = controlArchivePath
				dataArchivePaths[i] = dataArchivePath

				return nil
			})
		}

		err := g.Wait()

		if bar != nil {
			if err != nil {
				bar.Abort(true)
			} else {
				bar.SetTotal(bar.Current(), true)
			}
			bar.Wait()
		}

		if err != nil {
			return "", nil, fmt.Errorf("failed to decompress packages: %w", err)
		}
	}

	dpkgConfArchiveFile, err := os.Create(filepath.Join(tempDir, "dpkg-conf.tar"))
	if err != nil {
		return "", nil, fmt.Errorf("failed to create dpkg tar archive: %w", err)
	}
	defer dpkgConfArchiveFile.Close()

	tw := tar.NewWriter(dpkgConfArchiveFile)
	defer tw.Close()

	var packages []types.Package
	{
		var bar *mpb.Bar
		if progress != nil {
			bar = progress.AddBar(int64(len(packagePaths)),
				mpb.PrependDecorators(
					decor.Name("Extracting: "),
					decor.CountersNoUnit("%d / %d"),
				),
				mpb.AppendDecorators(
					decor.Percentage(),
				),
			)
		}

		for i := range packagePaths {
			slog.Debug("Extracting control archive",
				slog.String("path", filepath.Base(controlArchivePaths[i])))

			controlArchiveFile, err := os.Open(controlArchivePaths[i])
			if err != nil {
				if bar != nil {
					bar.Abort(true)
					bar.Wait()
				}

				return "", nil, fmt.Errorf("failed to open control archive file: %w", err)
			}

			pkg, err := extractControlArchive(tw, controlArchiveFile)
			_ = controlArchiveFile.Close()
			if err != nil {
				if bar != nil {
					bar.Abort(true)
					bar.Wait()
				}

				return "", nil, fmt.Errorf("failed to extract control archive: %w", err)
			}

			pkg.Status = []string{"install", "ok", "unpacked"}
			packages = append(packages, *pkg)

			// Get the list of files in the data archive.
			dataArchiveFile, err := os.Open(dataArchivePaths[i])
			if err != nil {
				if bar != nil {
					bar.Abort(true)
					bar.Wait()
				}

				return "", nil, fmt.Errorf("failed to open data archive file: %w", err)
			}

			filesList, err := getDataArchiveFileList(dataArchiveFile)
			_ = dataArchiveFile.Close()
			if err != nil {
				if bar != nil {
					bar.Abort(true)
					bar.Wait()
				}

				return "", nil, fmt.Errorf("failed to get data archive file list: %w", err)
			}

			filesListContent := strings.NewReader(strings.Join(filesList, "\n") + "\n")

			// Write the files list to the dpkg info directory.
			hdr := &tar.Header{
				Name: filepath.Join("var/lib/dpkg/info", fmt.Sprintf("%s.list", pkg.Name)),
				Mode: 0o644,
				Size: int64(filesListContent.Len()),
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return "", nil, fmt.Errorf("failed to write tar header: %w", err)
			}

			if _, err := io.Copy(tw, filesListContent); err != nil {
				return "", nil, fmt.Errorf("failed to write files list to tar archive: %w", err)
			}

			if bar != nil {
				bar.Increment()
			}
		}
	}

	// Write the dpkg status file.
	var buf bytes.Buffer
	if err := deb822.Marshal(&buf, packages); err != nil {
		return "", nil, fmt.Errorf("failed to marshal packages: %w", err)
	}

	hdr := &tar.Header{
		Name: "var/lib/dpkg/status",
		Size: int64(buf.Len()),
		Mode: 0o644,
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return "", nil, fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := io.Copy(tw, bytes.NewReader(buf.Bytes())); err != nil {
		return "", nil, fmt.Errorf("failed to write packages to tar archive: %w", err)
	}

	return dpkgConfArchiveFile.Name(), dataArchivePaths, nil
}

func decompressPackage(tempDir string, packagePath string) (string, string, error) {
	pf, err := os.Open(packagePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open package file: %w", err)
	}
	defer pf.Close()

	debFS, err := arfs.Open(pf)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse debian package: %w", err)
	}

	// Check that the package is a debian 2.0 format package.
	debianBinaryFile, err := debFS.Open("debian-binary")
	if err != nil {
		return "", "", fmt.Errorf("failed to open debian-binary file: %w", err)
	}

	debianBinary, err := io.ReadAll(debianBinaryFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read debian-binary file: %w", err)
	}

	if string(debianBinary) != "2.0\n" {
		return "", "", fmt.Errorf("unsupported debian package version: %s", debianBinary)
	}

	// Look for control and data archives in the debian package.
	entries, err := debFS.ReadDir(".")
	if err != nil {
		return "", "", fmt.Errorf("failed to read debian package: %w", err)
	}

	var controlArchivePath, dataArchivePath string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "control.tar") {
			controlArchivePath = entry.Name()
		} else if strings.HasPrefix(entry.Name(), "data.tar") {
			dataArchivePath = entry.Name()
		}
	}
	if controlArchivePath == "" {
		return "", "", fmt.Errorf("failed to find control archive in debian package")
	}
	if dataArchivePath == "" {
		return "", "", fmt.Errorf("failed to find data archive in debian package")
	}

	// Decompress the control archive.
	slog.Debug("Decompressing control archive",
		slog.String("packagePath", packagePath),
		slog.String("controlArchivePath", filepath.Base(controlArchivePath)))

	controlArchive, err := debFS.Open(controlArchivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open control archive: %w", err)
	}

	dr, err := compressmagic.NewReader(controlArchive)
	if err != nil {
		return "", "", fmt.Errorf("failed to decompress control archive: %w", err)
	}

	decompressedControlArchivePath := filepath.Join(tempDir, strings.TrimSuffix(filepath.Base(packagePath), ".deb")+"_control.tar")

	decompressedControlArchive, err := os.Create(decompressedControlArchivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create decompressed control archive: %w", err)
	}
	defer decompressedControlArchive.Close()

	if _, err := io.Copy(decompressedControlArchive, dr); err != nil {
		return "", "", fmt.Errorf("failed to write to decompressed control archive: %w", err)
	}

	// Decompress the data archive.
	slog.Debug("Decompressing data archive",
		slog.String("packagePath", packagePath),
		slog.String("dataArchivePath", filepath.Base(dataArchivePath)))

	dataArchive, err := debFS.Open(dataArchivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open data archive: %w", err)
	}

	dr, err = compressmagic.NewReader(dataArchive)
	if err != nil {
		return "", "", fmt.Errorf("failed to decompress data archive: %w", err)
	}

	decompressedDataArchivePath := filepath.Join(tempDir, strings.TrimSuffix(filepath.Base(packagePath), ".deb")+"_data.tar")

	decompressedDataArchive, err := os.Create(decompressedDataArchivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create decompressed data archive: %w", err)
	}

	if _, err := io.Copy(decompressedDataArchive, dr); err != nil {
		return "", "", fmt.Errorf("failed to write to decompressed data archive: %w", err)
	}

	return decompressedControlArchivePath, decompressedDataArchivePath, nil
}

func extractControlArchive(tw *tar.Writer, controlArchiveFile *os.File) (*types.Package, error) {
	controlArchive, err := tarfs.Open(controlArchiveFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open control archive: %w", err)
	}

	controlFile, err := controlArchive.Open("control")
	if err != nil {
		return nil, fmt.Errorf("failed to open control file: %w", err)
	}
	defer controlFile.Close()

	// Parse the control file.
	decoder, err := deb822.NewDecoder(controlFile, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create control file decoder: %w", err)
	}

	var pkg types.Package
	if err := decoder.Decode(&pkg); err != nil {
		return nil, fmt.Errorf("failed to decode control file: %w", err)
	}

	// Walk the control archive.
	err = fs.WalkDir(controlArchive, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || path == "control" {
			return nil
		}

		// Open the file in the control archive.
		f, err := controlArchive.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s in control archive: %w", path, err)
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file in control archive: %w", err)
		}

		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		hdr.Name = filepath.Join("var/lib/dpkg/info", fmt.Sprintf("%s.%s", pkg.Name, path))

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("failed to write file content to tar archive: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk control archive: %w", err)
	}

	return &pkg, nil
}

func getDataArchiveFileList(dataArchiveFile *os.File) ([]string, error) {
	// Open the data archive as a tar archive.
	dataArchive, err := tarfs.Open(dataArchiveFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open data archive: %w", err)
	}

	var filesList []string
	err = fs.WalkDir(dataArchive, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}

		filesList = append(filesList, path)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk data archive: %w", err)
	}

	return filesList, nil
}
