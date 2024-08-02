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

package secondstage

import (
	"context"
	"fmt"
	"log/slog"

	latestrecipe "github.com/dpeckett/debco/internal/recipe/v1alpha1"
	"github.com/dpeckett/debco/internal/secondstage/slimify"
	"github.com/dpeckett/debco/internal/secondstage/users"
)

func Provision(ctx context.Context, recipe *latestrecipe.Recipe) error {
	if recipe.Options != nil && recipe.Options.Slimify {
		slog.Info("Slimifying image")

		if err := slimify.Slimify(); err != nil {
			return fmt.Errorf("failed to slimify: %w", err)
		}
	}

	for _, groupConf := range recipe.Groups {
		slog.Info("Creating or updating group", slog.String("name", groupConf.Name))

		group := users.Group{
			Name:    groupConf.Name,
			GID:     groupConf.GID,
			Members: groupConf.Members,
			System:  groupConf.System,
		}

		if err := users.CreateOrUpdateGroup(group); err != nil {
			return fmt.Errorf("failed to create group %q: %w", groupConf.Name, err)
		}
	}

	for _, userConf := range recipe.Users {
		slog.Info("Creating or updating user", slog.String("name", userConf.Name))

		user := users.User{
			Name:     userConf.Name,
			UID:      userConf.UID,
			Groups:   userConf.Groups,
			HomeDir:  userConf.HomeDir,
			Shell:    userConf.Shell,
			Password: userConf.Password,
			System:   userConf.System,
		}

		if err := users.CreateOrUpdateUser(user); err != nil {
			return fmt.Errorf("failed to create or update user %q: %w", userConf.Name, err)
		}
	}

	return nil
}
