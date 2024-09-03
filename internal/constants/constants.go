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

package constants

var (
	// BuildKitImage is the image used for the BuildKit daemon.
	BuildKitImage = "docker.io/moby/buildkit:v0.13.2"
	// During the building process we use the upstream apt repository to fetch
	// the second stage debco binary for bootstrapping the system.
	UpstreamAPTURL      = "https://apt.pecke.tt"
	UpstreamAPTSignedBy = "https://apt.pecke.tt/signing_key.asc"
	// TelemetryURL is the URL to send anonymized telemetry data to.
	TelemetryURL = "https://telemetry.pecke.tt"
	// Version will be populated during build time.
	Version = "dev"
)
