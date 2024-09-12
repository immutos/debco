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

package database

import (
	"sync"

	debtypes "github.com/dpeckett/deb822/types"
	"github.com/dpeckett/deb822/types/version"
	"github.com/immutos/debco/internal/types"

	"github.com/google/btree"
)

// PackageDB is a package database.
type PackageDB struct {
	mu   sync.RWMutex
	tree *btree.BTree
}

// NewPackageDB creates a new package database.
func NewPackageDB() *PackageDB {
	return &PackageDB{
		tree: btree.New(2),
	}
}

// Len returns the total number of packages in the database.
func (db *PackageDB) Len() int {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var count int
	db.tree.Ascend(func(item btree.Item) bool {
		pkg := item.(types.Package)

		if !pkg.IsVirtual {
			count++
		}

		return true
	})

	return count
}

// Add adds a package to the database.
func (db *PackageDB) Add(pkg types.Package) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.addPackage(pkg)
}

// AddAll adds multiple packages to the database.
func (db *PackageDB) AddAll(packageList []types.Package) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, pkg := range packageList {
		db.addPackage(pkg)
	}
}

func (db *PackageDB) addPackage(pkg types.Package) {
	// Do we already have this package?
	if existing := db.tree.Get(pkg); existing != nil {
		existing := existing.(types.Package)

		// Append the url to the existing package (if an identical url does not already exist).
		for _, url := range pkg.URLs {
			var found bool
			for _, existingURL := range existing.URLs {
				if url == existingURL {
					found = true
					break
				}
			}
			if !found {
				existing.URLs = append(existing.URLs, url)
			}
		}

		pkg = existing
	}

	db.tree.ReplaceOrInsert(pkg)

	// Does this package provide any virtual packages?
	if len(pkg.Provides.Relations) > 0 {
		for _, rel := range pkg.Provides.Relations {
			for _, possi := range rel.Possibilities {
				virtualPkg := types.Package{
					Package:   debtypes.Package{Name: possi.Name},
					IsVirtual: true,
				}

				if possi.Version != nil {
					virtualPkg.Version = possi.Version.Version
				}

				// Do we already have a virtual package?
				if existing := db.tree.Get(virtualPkg); existing != nil {
					virtualPkg = existing.(types.Package)
				}

				// Make sure the package is not already in the providers list.
				var found bool
				for _, provider := range virtualPkg.Providers {
					if provider.Compare(pkg) == 0 {
						found = true
						break
					}
				}

				// Add the package to the providers list (if it is not already there).
				if !found {
					virtualPkg.Providers = append(virtualPkg.Providers, pkg)
					db.tree.ReplaceOrInsert(virtualPkg)
				}
			}
		}
	}
}

// Remove removes a package from the database.
func (db *PackageDB) Remove(pkg types.Package) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.tree.Delete(pkg)

	// If the package provides any virtual packages, update the providers.
	if len(pkg.Provides.Relations) > 0 {
		for _, rel := range pkg.Provides.Relations {
			for _, possi := range rel.Possibilities {
				virtualPkg := types.Package{
					Package:   debtypes.Package{Name: possi.Name},
					IsVirtual: true,
				}

				if possi.Version != nil {
					virtualPkg.Version = possi.Version.Version
				}

				if virtualPkg := db.tree.Get(virtualPkg); virtualPkg != nil {
					virtualPkg := virtualPkg.(types.Package)

					// Remove the package from the providers list.
					var updatedProviders []types.Package
					for _, provider := range virtualPkg.Providers {
						if provider.Compare(pkg) != 0 {
							updatedProviders = append(updatedProviders, provider)
						}
					}
					virtualPkg.Providers = updatedProviders

					// If there are no more providers, remove the virtual package.
					if len(virtualPkg.Providers) == 0 {
						db.tree.Delete(virtualPkg)
					} else {
						db.tree.ReplaceOrInsert(virtualPkg)
					}
				}
			}
		}
	}
}

// ForEach iterates over each package in the database.
// If the provided function returns an error, the iteration will stop.
func (db *PackageDB) ForEach(fn func(pkg types.Package) error) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var err error
	db.tree.Ascend(func(item btree.Item) bool {
		pkg := item.(types.Package)

		if !pkg.IsVirtual {
			err = fn(pkg)
		}
		return err == nil
	})
	return err
}

// Get returns all packages that match the provided name.
func (db *PackageDB) Get(name string) (packageList []types.Package) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	db.tree.AscendGreaterOrEqual(types.Package{
		Package: debtypes.Package{Name: name},
	}, func(item btree.Item) bool {
		pkg := item.(types.Package)

		if pkg.Package.Name != name {
			return false
		}

		packageList = append(packageList, pkg)

		return true
	})
	return
}

// StrictlyEarlier returns all packages that match the provided name and are
// strictly earlier than the provided version.
func (db *PackageDB) StrictlyEarlier(name string, version version.Version) (packageList []types.Package) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	db.tree.DescendLessOrEqual(types.Package{
		Package: debtypes.Package{Name: name, Version: version},
	}, func(item btree.Item) bool {
		pkg := item.(types.Package)

		if pkg.Package.Name != name {
			return false
		}

		// Skip the package if it is the same version (since we want strictly earlier)
		if pkg.Version.Compare(version) == 0 {
			return true
		}

		packageList = append(packageList, pkg)

		return true
	})
	return
}

// EarlierOrEqual returns all packages that match the provided name and are
// earlier or equal to the provided version.
func (db *PackageDB) EarlierOrEqual(name string, version version.Version) (packageList []types.Package) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	db.tree.DescendLessOrEqual(types.Package{
		Package: debtypes.Package{Name: name, Version: version},
	}, func(item btree.Item) bool {
		pkg := item.(types.Package)

		if pkg.Package.Name != name {
			return false
		}

		packageList = append(packageList, pkg)

		return true
	})
	return
}

// ExactlyEqual returns the package that matches the provided name and version.
func (db *PackageDB) ExactlyEqual(name string, version version.Version) (*types.Package, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var foundPackage *types.Package
	db.tree.AscendGreaterOrEqual(types.Package{
		Package: debtypes.Package{Name: name, Version: version},
	}, func(item btree.Item) bool {
		pkg := item.(types.Package)

		if pkg.Package.Name != name {
			return false
		}

		if pkg.Version.Compare(version) == 0 {
			foundPackage = &pkg
		}

		return false
	})
	return foundPackage, foundPackage != nil
}

// LaterOrEqual returns all packages that match the provided name and are
// later or equal to the provided version.
func (db *PackageDB) LaterOrEqual(name string, version version.Version) (packageList []types.Package) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	db.tree.AscendGreaterOrEqual(types.Package{
		Package: debtypes.Package{Name: name, Version: version},
	}, func(item btree.Item) bool {
		pkg := item.(types.Package)

		if pkg.Package.Name != name {
			return false
		}

		packageList = append(packageList, pkg)

		return true
	})
	return
}

// StrictlyLater returns all packages that match the provided name and are
// strictly later than the provided version.
func (db *PackageDB) StrictlyLater(name string, version version.Version) (packageList []types.Package) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	db.tree.AscendGreaterOrEqual(types.Package{
		Package: debtypes.Package{Name: name, Version: version},
	}, func(item btree.Item) bool {
		pkg := item.(types.Package)

		if pkg.Package.Name != name {
			return false
		}

		// Skip the package if it is the same version (since we want strictly later)
		if pkg.Version.Compare(version) == 0 {
			return true
		}

		packageList = append(packageList, pkg)

		return true
	})
	return
}
