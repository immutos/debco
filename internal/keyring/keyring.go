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

package keyring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// Load reads an OpenPGP keyring from a file or URL.
func Load(ctx context.Context, key string) (openpgp.EntityList, error) {
	if len(key) == 0 {
		return openpgp.EntityList{}, nil
	}

	// If the key is a URL, download it.
	if strings.Contains(key, "://") {
		slog.Debug("Downloading key", slog.String("url", key))

		keyURL, err := url.Parse(key)
		if err != nil {
			return nil, err
		}

		if keyURL.Scheme != "https" {
			return nil, errors.New("key URL must be HTTPS")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, keyURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to download key: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download key: %s", resp.Status)
		}

		// ReadArmoredKeyRing() doesn't read the entire response body, so we need
		// to do it ourselves (so that response caching will work as expected).
		keyringData, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		return openpgp.ReadArmoredKeyRing(bytes.NewReader(keyringData))
	} else { // If the key is a file, open it.
		slog.Debug("Reading key file", slog.String("path", key))

		f, err := os.Open(key)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		return openpgp.ReadArmoredKeyRing(f)
	}
}
