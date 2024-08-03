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

package hashreader

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
)

// HashReader is a wrapper around an io.Reader that calculates the SHA-256 hash of the read data.
type HashReader struct {
	reader io.Reader
	hasher hash.Hash
}

// NewReader creates a new HashReader.
func NewReader(r io.Reader) *HashReader {
	hasher := sha256.New()
	return &HashReader{
		reader: io.TeeReader(r, hasher),
		hasher: hasher,
	}
}

// Read reads from the underlying reader and updates the hash.
func (hr *HashReader) Read(p []byte) (int, error) {
	return hr.reader.Read(p)
}

// Verify returns true if the calculated hash matches the expected hash.
func (hr *HashReader) Verify(expected string) error {
	expectedHash, err := hex.DecodeString(expected)
	if err != nil {
		return err
	}

	if !hmac.Equal(hr.hasher.Sum(nil), expectedHash) {
		return errors.New("hash mismatch")
	}

	return nil
}
