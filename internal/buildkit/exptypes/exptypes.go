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

// Package exptypes from buildkit is not available in the Debian bookworm
// docker-dev package. Remove this as soon as the package is available.
package exptypes

import ocispecs "github.com/opencontainers/image-spec/specs-go/v1"

const (
	ExporterImageConfigKey = "containerimage.config"
	ExporterPlatformsKey   = "refs.platforms"
	OptKeyRewriteTimestamp = "rewrite-timestamp"
	OptKeySourceDateEpoch  = "source-date-epoch"
)

type Platforms struct {
	Platforms []Platform
}

type Platform struct {
	ID       string
	Platform ocispecs.Platform
}
