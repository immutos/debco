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

package recipe

import (
	"fmt"
	"io"

	recipetypes "github.com/immutos/debco/internal/recipe/types"
	latestrecipe "github.com/immutos/debco/internal/recipe/v1alpha1"
	"gopkg.in/yaml.v3"
)

// FromYAML reads the given reader and returns a recipe object.
func FromYAML(r io.Reader) (*latestrecipe.Recipe, error) {
	recipeBytes, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe from reader: %w", err)
	}

	var typeMeta recipetypes.TypeMeta
	if err := yaml.Unmarshal(recipeBytes, &typeMeta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal type meta from recipe file: %w", err)
	}

	var versionedRecipe recipetypes.Typed
	switch typeMeta.APIVersion {
	case latestrecipe.APIVersion:
		versionedRecipe, err = latestrecipe.GetByKind(typeMeta.Kind)
	default:
		return nil, fmt.Errorf("unsupported api version: %s", typeMeta.APIVersion)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get recipe by kind %q: %w", typeMeta.Kind, err)
	}

	if err := yaml.Unmarshal(recipeBytes, versionedRecipe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recipe from recipe file: %w", err)
	}

	versionedRecipe, err = MigrateToLatest(versionedRecipe)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate recipe: %w", err)
	}

	return versionedRecipe.(*latestrecipe.Recipe), nil
}

// ToYAML writes the given recipe object to the given writer.
func ToYAML(w io.Writer, versionedRecipe recipetypes.Typed) error {
	versionedRecipe.PopulateTypeMeta()

	if err := yaml.NewEncoder(w).Encode(versionedRecipe); err != nil {
		return fmt.Errorf("failed to marshal recipe: %w", err)
	}

	return nil
}

// MigrateToLatest migrates the given recipe object to the latest version.
func MigrateToLatest(versionedRecipe recipetypes.Typed) (recipetypes.Typed, error) {
	switch recipe := versionedRecipe.(type) {
	case *latestrecipe.Recipe:
		// Nothing to do, already at the latest version.
		return recipe, nil
	default:
		return nil, fmt.Errorf("unsupported recipe version: %s", recipe.GetAPIVersion())
	}
}
