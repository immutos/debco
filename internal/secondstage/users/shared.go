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
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"

	cp "github.com/otiai10/copy"
)

var validNameRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{0,31}$`)

func updateFile(path string, perm fs.FileMode, updateFunc func(*lineReader) (string, error)) error {
	if err := cp.Copy(path, path+"-", cp.Options{Sync: true}); err != nil {
		return fmt.Errorf("failed to backup %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	contents, err := updateFunc(&lineReader{bufio.NewReader(f)})
	if err != nil {
		return err
	}

	if err := f.Truncate(0); err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	_, err = f.WriteString(contents)
	return err
}

type lineReader struct {
	*bufio.Reader
}

func (r *lineReader) nextLine() (string, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return "", err
		}

		if len(line) == 0 {
			return "", io.EOF
		}
	}

	return strings.TrimSpace(string(line)), nil
}
