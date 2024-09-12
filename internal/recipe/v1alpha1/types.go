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

package v1alpha1

import (
	"fmt"

	"github.com/immutos/debco/internal/recipe/types"
)

const APIVersion = "debco/v1alpha1"

type Recipe struct {
	types.TypeMeta `yaml:",inline"`
	// Options contains configuration options for the image.
	Options *OptionsConfig `yaml:"options,omitempty"`
	// Sources is a list of apt repositories to use for downloading packages.
	Sources []SourceConfig `yaml:"sources"`
	// Packages is the package configuration.
	Packages PackagesConfig `yaml:"packages"`
	// Groups is a list of groups to create.
	Groups []GroupConfig `yaml:"groups,omitempty"`
	// Users is a list of users to create.
	Users []UserConfig `yaml:"users,omitempty"`
	// Container is the OCI image configuration.
	Container *ContainerConfig `yaml:"container,omitempty"`
}

// OptionsConfig contains configuration options for the image.
type OptionsConfig struct {
	// OmitRequired specifies whether to omit priority required packages from the installation.
	// By default, any packages marked as priority required will be installed.
	OmitRequired bool `yaml:"omitRequired,omitempty"`
	// Slimify specifies whether to slimify the image by removing unnecessary files.
	Slimify bool `yaml:"slimify,omitempty"`
}

// SourceConfig is the configuration for an apt repository.
type SourceConfig struct {
	// URL is the URL of the repository.
	URL string `yaml:"url"`
	// Signed by is a public key URL (https) or file path to use for verifying the repository.
	SignedBy string `yaml:"signedBy"`
	// Distribution specifies the Debian distribution name (e.g., bullseye, buster)
	// or class (e.g., stable, testing). If not specified, defaults to "stable".
	Distribution string `yaml:"distribution,omitempty"`
	// Components is a list of components to use from the repository.
	// If not specified, defaults to ["main"].
	Components []string `yaml:"components,omitempty"`
}

// PackagesConfig is the configuration for packages.
type PackagesConfig struct {
	// Include is a list of packages to install.
	Include []string `yaml:"include,omitempty"`
	// Exclude is a list of packages to exclude from installation.
	Exclude []string `yaml:"exclude,omitempty"`
}

// GroupConfig is the configuration for a group.
type GroupConfig struct {
	// Name is the name of the group.
	Name string `yaml:"name"`
	// GID is the group ID to use for the group.
	GID *uint `yaml:"gid,omitempty"`
	// Members is a list of users to add to the group.
	Members []string `yaml:"members,omitempty"`
	// System specifies whether the group is a system group.
	System bool `yaml:"system,omitempty"`
}

// UserConfig is the configuration for a user.
type UserConfig struct {
	// Name is the name of the user.
	Name string `yaml:"name"`
	// UID is the user ID to use for the user.
	UID *uint `yaml:"uid,omitempty"`
	// Groups is a list of groups to add the user to.
	// The first group in the list will be treated as the users primary group.
	Groups []string `yaml:"groups,omitempty"`
	// HomeDir is the home directory for the user.
	HomeDir string `yaml:"homeDir,omitempty"`
	// Shell is the shell for the user.
	Shell string `yaml:"shell,omitempty"`
	// Password is the optional password for the user.
	// If not specified, password authentication will be disabled.
	Password string `yaml:"password,omitempty"`
	// System specifies whether the user is a system user.
	System bool `yaml:"system,omitempty"`
}

// ContainerConfig is the configuration for the container.
type ContainerConfig struct {
	// User defines the username or UID which the process in the container should run as.
	User string `yaml:"user,omitempty"`
	// ExposedPorts a set of ports to expose from a container running this image.
	ExposedPorts map[string]struct{} `yaml:"exposedPorts,omitempty"`
	// Env is a list of additional environment variables to be used in a container.
	Env []string `yaml:"env,omitempty"`
	// Entrypoint defines a list of arguments to use as the command to execute when
	// the container starts.
	Entrypoint []string `yaml:"entrypoint,omitempty"`
	// Cmd defines the default arguments to the entrypoint of the container.
	Cmd []string `yaml:"cmd,omitempty"`
	// Volumes is a set of directories describing where the process is likely write
	// data specific to a container instance.
	Volumes map[string]struct{} `yaml:"volumes,omitempty"`
	// WorkingDir sets the current working directory of the entrypoint process in the container.
	WorkingDir string `yaml:"workingDir,omitempty"`
	// Labels contains arbitrary metadata for the container.
	Labels map[string]string `yaml:"labels,omitempty"`
	// StopSignal contains the system call signal that will be sent to the container to exit.
	StopSignal string `yaml:"stopSignal,omitempty"`
}

func (c *Recipe) GetAPIVersion() string {
	return APIVersion
}

func (c *Recipe) GetKind() string {
	return "Recipe"
}

func (c *Recipe) PopulateTypeMeta() {
	c.TypeMeta = types.TypeMeta{
		APIVersion: APIVersion,
		Kind:       "Recipe",
	}
}

func GetByKind(kind string) (types.Typed, error) {
	switch kind {
	case "Recipe":
		return &Recipe{}, nil
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}
}
