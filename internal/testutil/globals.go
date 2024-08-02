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

package testutil

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
)

func SetupGlobals(t *testing.T) {
	var buf bytes.Buffer
	h := &bridge{
		t:   t,
		buf: &buf,
		mu:  &sync.Mutex{},
		Handler: slog.NewTextHandler(&buf, &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelDebug,
		}),
	}

	slog.SetDefault(slog.New(h))
}
