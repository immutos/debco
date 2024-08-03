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

package diskcache

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/rogpeppe/go-internal/cache"
)

// DiskCache is a cache that stores http responses on disk.
type DiskCache struct {
	*cache.Cache
	namespace string
}

// NewDiskCache creates a new cache that stores responses in the given directory.
// The namespace is used to separate different caches in the same directory.
func NewDiskCache(dir, namespace string) (*DiskCache, error) {
	c, err := cache.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("error opening cache: %w", err)
	}

	c.Trim()

	return &DiskCache{
		Cache:     c,
		namespace: namespace,
	}, nil
}

func (c *DiskCache) Get(key string) ([]byte, bool) {
	responseBytes, _, err := c.Cache.GetBytes(c.getActionID(key))
	if err != nil {
		if !(errors.Is(err, os.ErrNotExist) || err.Error() == "cache entry not found") {
			slog.Warn("Error getting cached response",
				slog.String("key", key), slog.Any("error", err))
		} else {
			slog.Debug("Cache miss", slog.String("key", key))
		}

		return nil, false
	}

	slog.Debug("Cache hit", slog.String("key", key))

	return responseBytes, true
}

func (c *DiskCache) Set(key string, responseBytes []byte) {
	slog.Debug("Storing cached response", slog.String("key", key))

	if err := c.Cache.PutBytes(c.getActionID(key), responseBytes); err != nil {
		slog.Warn("Error setting cached response", slog.Any("error", err))
	}
}

func (c *DiskCache) Delete(key string) {}

func (c *DiskCache) getActionID(key string) cache.ActionID {
	h := cache.NewHash(c.namespace)
	_, _ = h.Write([]byte(key))
	return h.Sum()
}
