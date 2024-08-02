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

package slimify

import (
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/dockerignore"
	"github.com/moby/patternmatcher"
)

//go:embed embed/.slimify
var dotSlimify []byte

var excludedDirs = map[string]bool{
	"/dev":  true,
	"/proc": true,
	"/sys":  true,
	"/tmp":  true,
}

// Slimify the image by removing unnecessary files.
func Slimify() error {
	patterns, err := dockerignore.ReadAll(bytes.NewReader(dotSlimify))
	if err != nil {
		return fmt.Errorf("failed to read patterns: %w", err)
	}

	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return fmt.Errorf("failed to create pattern matcher: %w", err)
	}

	// First walk the root filesystem and collect paths to remove.
	var pathsToRemove []string
	err = filepath.WalkDir("/", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				slog.Warn("Skipping", "path", path, "error", err)
				return nil
			}

			return err
		}

		// Skip special directories.
		if excludedDirs[path] {
			return fs.SkipDir
		}

		matches, err := pm.MatchesOrParentMatches(strings.TrimPrefix(path, "/"))
		if err != nil {
			return fmt.Errorf("failed to match %s: %w", path, err)
		}

		if matches {
			pathsToRemove = append(pathsToRemove, path)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk root filesystem: %w", err)
	}

	// Sort the paths in reverse order so that we remove files before directories.
	sort.Slice(pathsToRemove, func(i, j int) bool {
		return len(pathsToRemove[i]) > len(pathsToRemove[j])
	})

	// Remove the paths.
	for _, path := range pathsToRemove {
		fi, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", path, err)
		}

		if fi.IsDir() {
			empty, err := isDirEmpty(path)
			if err != nil {
				return fmt.Errorf("failed to check if %s is empty: %w", path, err)
			}

			if !empty {
				continue
			}
		}

		slog.Debug("Removing", slog.String("path", path))

		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}

	return nil
}

func isDirEmpty(path string) (bool, error) {
	var filenames []string
	err := filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			filenames = append(filenames, path)
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	return len(filenames) == 0, nil
}
