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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/containerd/containerd/platforms"
	"github.com/dpeckett/deb822/types/arch"
	"github.com/dpeckett/debco/internal/buildkit"
	"github.com/dpeckett/debco/internal/constants"
	"github.com/dpeckett/debco/internal/database"
	"github.com/dpeckett/debco/internal/diskcache"
	"github.com/dpeckett/debco/internal/hashreader"
	"github.com/dpeckett/debco/internal/recipe"
	latestrecipe "github.com/dpeckett/debco/internal/recipe/v1alpha1"
	"github.com/dpeckett/debco/internal/resolve"
	"github.com/dpeckett/debco/internal/secondstage"
	"github.com/dpeckett/debco/internal/source"
	"github.com/dpeckett/debco/internal/types"
	"github.com/dpeckett/debco/internal/unpack"
	"github.com/dpeckett/debco/internal/util"
	"github.com/gregjones/httpcache"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/sync/errgroup"
)

func main() {
	defaultCacheDir, _ := xdg.CacheFile("debco")
	defaultStateDir, _ := xdg.StateFile("debco")

	persistentFlags := []cli.Flag{
		&cli.GenericFlag{
			Name:  "log-level",
			Usage: "Set the log verbosity level",
			Value: util.FromSlogLevel(slog.LevelInfo),
		},
		&cli.StringFlag{
			Name:   "cache-dir",
			Usage:  "Directory to store the cache",
			Value:  defaultCacheDir,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "state-dir",
			Usage:  "Directory to store application state",
			Value:  defaultStateDir,
			Hidden: true,
		},
	}

	initLogger := func(c *cli.Context) error {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: (*slog.Level)(c.Generic("log-level").(*util.LevelFlag)),
		})))

		return nil
	}

	initCacheDir := func(c *cli.Context) error {
		cacheDir := c.String("cache-dir")
		if cacheDir == "" {
			return fmt.Errorf("no cache directory specified")
		}

		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}

		return nil
	}

	initStateDir := func(c *cli.Context) error {
		stateDir := c.String("state-dir")
		if stateDir == "" {
			return fmt.Errorf("no state directory specified")
		}

		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			return fmt.Errorf("failed to create state directory: %w", err)
		}

		return nil
	}

	app := &cli.App{
		Name:    "debco",
		Usage:   "A declarative Debian base system builder",
		Version: constants.Version,
		Commands: []*cli.Command{
			{
				Name:  "build",
				Usage: "Build a Debian base system image",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:     "filename",
						Aliases:  []string{"f"},
						Usage:    "Recipe file to use",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output OCI image archive",
						Value:   "debian-image.tar",
					},
					&cli.StringFlag{
						Name:    "platform",
						Aliases: []string{"p"},
						Usage:   "Target platform(s) in the 'os/arch' format",
						Value:   "linux/" + runtime.GOARCH,
					},
					&cli.StringSliceFlag{
						Name:    "tag",
						Aliases: []string{"t"},
						Usage:   "Name and optionally a tag for the image in the 'name:tag' format",
						Value:   cli.NewStringSlice(),
					},
					&cli.BoolFlag{
						Name:  "dev",
						Usage: "Enable development mode",
					},
				}, persistentFlags...),
				Before: util.BeforeAll(initLogger, initCacheDir, initStateDir),
				Action: func(c *cli.Context) error {
					// Cache all HTTP responses on disk.
					cache, err := diskcache.NewDiskCache(c.String("cache-dir"), "http")
					if err != nil {
						return fmt.Errorf("failed to create disk cache: %w", err)
					}

					// Use the disk cache for all HTTP requests.
					http.DefaultClient = &http.Client{
						Transport: httpcache.NewTransport(cache),
					}

					// A temporary directory used during image building.
					tempDir, err := os.MkdirTemp("", "debco-*")
					if err != nil {
						return fmt.Errorf("failed to create temporary directory: %w", err)
					}
					defer func() {
						_ = os.RemoveAll(tempDir)
					}()

					// Mutual TLS certificates for the BuildKit daemon.
					certsDir := filepath.Join(c.String("state-dir"), "certs")
					if err := os.MkdirAll(certsDir, 0o700); err != nil {
						return fmt.Errorf("failed to create certs directory: %w", err)
					}

					// Load the recipe file.
					recipeFile, err := os.Open(c.String("filename"))
					if err != nil {
						return fmt.Errorf("failed to open recipe file: %w", err)
					}
					defer recipeFile.Close()

					recipe, err := recipe.FromYAML(recipeFile)
					if err != nil {
						return fmt.Errorf("failed to read recipe: %w", err)
					}

					// Start the BuildKit daemon.
					b := buildkit.New("debco", certsDir)
					if err := b.StartDaemon(c.Context); err != nil {
						return fmt.Errorf("failed to start buildkit daemon: %w", err)
					}

					// If running in development mode, use the current debco binary as the
					// second stage binary.
					var secondStageBinaryPath string
					if c.Bool("dev") {
						secondStageBinaryPath, err = os.Executable()
						if err != nil {
							return fmt.Errorf("failed to get executable path: %w", err)
						}
					}

					buildOpts := buildkit.BuildOptions{
						OCIArchivePath:        c.String("output"),
						RecipePath:            c.String("filename"),
						SecondStageBinaryPath: secondStageBinaryPath,
						ImageConf:             toOCIImageConfig(recipe),
						Tags:                  c.StringSlice("tag"),
					}

					for _, platformStr := range strings.Split(c.String("platform"), ",") {
						platform, err := platforms.Parse(platformStr)
						if err != nil {
							return fmt.Errorf("failed to parse platform: %w", err)
						}

						if platform.OS != "linux" {
							return fmt.Errorf("unsupported OS: %s", platform.OS)
						}

						slog.Info("Building image", slog.String("platform", platforms.Format(platform)))

						slog.Info("Loading packages")

						var packageDB *database.PackageDB
						packageDB, sourceDateEpoch, err := loadPackageDB(c.Context, recipe, platform)
						if err != nil {
							return err
						}

						if sourceDateEpoch.After(buildOpts.SourceDateEpoch) {
							buildOpts.SourceDateEpoch = sourceDateEpoch
						}

						var requiredNameVersions []string

						// By default, install the debco binary (for second-stage provisioning).
						if !c.Bool("dev") {
							requiredNameVersions = append(requiredNameVersions, "debco")
						}

						// By default, install all priority required packages.
						if !(recipe.Options != nil && recipe.Options.OmitRequired) {
							_ = packageDB.ForEach(func(pkg types.Package) error {
								if pkg.Priority == "required" {
									requiredNameVersions = append(requiredNameVersions, pkg.Package.Name)
								}

								return nil
							})
						}

						slog.Info("Resolving selected packages")

						selectedDB, err := resolve.Resolve(packageDB,
							append(requiredNameVersions, recipe.Packages.Include...),
							recipe.Packages.Exclude)
						if err != nil {
							return err
						}

						platformTempDir := filepath.Join(tempDir, strings.ReplaceAll(platforms.Format(platform), "/", "-"))
						if err := os.MkdirAll(platformTempDir, 0o755); err != nil {
							return fmt.Errorf("failed to create platform temp directory: %w", err)
						}

						slog.Info("Downloading selected packages")

						packagePaths, err := downloadSelectedPackages(c.Context, platformTempDir, selectedDB)
						if err != nil {
							return err
						}

						slog.Info("Unpacking packages")

						dpkgConfArchivePath, dataArchivePaths, err := unpack.Unpack(c.Context, platformTempDir, packagePaths)
						if err != nil {
							return err
						}

						buildOpts.PlatformOpts = append(buildOpts.PlatformOpts, buildkit.PlatformBuildOptions{
							Platform:            platform,
							BuildContextDir:     platformTempDir,
							DpkgConfArchivePath: dpkgConfArchivePath,
							DataArchivePaths:    dataArchivePaths,
						})
					}

					slog.Info("Building multi-platform image", slog.String("output", c.String("output")))

					if err := b.Build(c.Context, buildOpts); err != nil {
						return fmt.Errorf("failed to build OCI image: %w", err)
					}

					return nil
				},
			},
			{
				Name:        "second-stage",
				Description: "Operations that will be run after the image is built",
				Hidden:      true,
				Subcommands: []*cli.Command{
					{
						// MergeUsr is a separate command as it needs to be run before
						// packages are configured.
						Name:        "merge-usr",
						Description: "Merge the /usr directory into the root filesystem",
						Flags:       persistentFlags,
						Before:      util.BeforeAll(initLogger),
						Action: func(_ *cli.Context) error {
							return secondstage.MergeUsr()
						},
					},
					{
						Name:        "provision",
						Description: "Set up the image with the requested recipe",
						Flags: append([]cli.Flag{
							&cli.StringFlag{
								Name:     "filename",
								Aliases:  []string{"f"},
								Usage:    "Recipe file to use",
								Required: true,
							},
						}, persistentFlags...),
						Before: util.BeforeAll(initLogger),
						Action: func(c *cli.Context) error {
							// Load the recipe file.
							recipeFile, err := os.Open(c.String("filename"))
							if err != nil {
								return fmt.Errorf("failed to open recipe file: %w", err)
							}
							defer recipeFile.Close()

							recipe, err := recipe.FromYAML(recipeFile)
							if err != nil {
								return fmt.Errorf("failed to read recipe: %w", err)
							}

							return secondstage.Provision(c.Context, recipe)
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Error", slog.Any("error", err))
		os.Exit(1)
	}
}

func loadPackageDB(ctx context.Context, recipe *latestrecipe.Recipe, platform ocispecs.Platform) (*database.PackageDB, time.Time, error) {
	var componentsMu sync.Mutex
	var components []source.Component

	var progress *mpb.Progress
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		progress = mpb.NewWithContext(ctx)
		defer progress.Shutdown()
	}

	{
		sourceConfs := append([]latestrecipe.SourceConfig{}, recipe.Sources...)

		if !(recipe.Options != nil && recipe.Options.OmitUpstreamAPT) {
			sourceConfs = append([]latestrecipe.SourceConfig{
				{
					URL:      constants.UpstreamAPTURL,
					SignedBy: constants.UpstreamAPTSignedBy,
					// Given debco is only linked to libc, this should be "fine".
					Distribution: "bookworm",
					Components:   []string{"stable"},
				},
			}, sourceConfs...)
		}

		g, ctx := errgroup.WithContext(ctx)

		var bar *mpb.Bar
		if progress != nil {
			bar = progress.AddBar(int64(len(sourceConfs)),
				mpb.PrependDecorators(
					decor.Name("Source: "),
					decor.CountersNoUnit("%d / %d"),
				),
				mpb.AppendDecorators(
					decor.Percentage(),
				),
			)
		}

		for _, sourceConf := range sourceConfs {
			sourceConf := sourceConf

			g.Go(func() error {
				defer func() {
					if bar != nil {
						bar.Increment()
					}
				}()

				s, err := source.NewSource(ctx, sourceConf)
				if err != nil {
					return fmt.Errorf("failed to create source: %w", err)
				}

				targetArch, err := arch.Parse(platform.Architecture)
				if err != nil {
					return fmt.Errorf("failed to parse target architecture: %w", err)
				}

				sourceComponents, err := s.Components(ctx, targetArch)
				if err != nil {
					return fmt.Errorf("failed to get components: %w", err)
				}

				componentsMu.Lock()
				components = append(components, sourceComponents...)
				componentsMu.Unlock()

				return nil
			})
		}

		err := g.Wait()

		if bar != nil {
			if err != nil {
				bar.Abort(true)
			} else {
				bar.SetTotal(bar.Current(), true)
			}
			bar.Wait()
		}

		if err != nil {
			return nil, time.Time{}, fmt.Errorf("failed to get components: %w", err)
		}
	}

	packageDB := database.NewPackageDB()

	var sourceDateEpoch time.Time
	{
		g, ctx := errgroup.WithContext(ctx)

		var bar *mpb.Bar
		if progress != nil {
			bar = progress.AddBar(int64(len(components)),
				mpb.PrependDecorators(
					decor.Name("Repository: "),
					decor.CountersNoUnit("%d / %d"),
				),
				mpb.AppendDecorators(
					decor.Percentage(),
				),
			)
		}

		for _, component := range components {
			component := component

			g.Go(func() error {
				defer func() {
					if bar != nil {
						bar.Increment()
					}
				}()

				componentPackages, lastUpdated, err := component.Packages(ctx)
				if err != nil {
					return fmt.Errorf("failed to get packages: %w", err)
				}

				if lastUpdated.After(sourceDateEpoch) {
					sourceDateEpoch = lastUpdated
				}

				packageDB.AddAll(componentPackages)

				return nil
			})
		}

		err := g.Wait()

		if bar != nil {
			if err != nil {
				bar.Abort(true)
			} else {
				bar.SetTotal(bar.Current(), true)
			}
			bar.Wait()
		}

		if err != nil {
			return nil, time.Time{}, fmt.Errorf("failed to get packages: %w", err)
		}
	}

	return packageDB, sourceDateEpoch, nil
}

func downloadSelectedPackages(ctx context.Context, tempDir string, selectedDB *database.PackageDB) ([]string, error) {
	var progress *mpb.Progress
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		progress = mpb.NewWithContext(ctx)
		defer progress.Shutdown()
	}

	var bar *mpb.Bar
	if progress != nil {
		bar = progress.AddBar(int64(selectedDB.Len()),
			mpb.PrependDecorators(
				decor.Name("Downloading: "),
				decor.CountersNoUnit("%d / %d"),
			),
			mpb.AppendDecorators(
				decor.Percentage(),
			),
		)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	var packagePathsMu sync.Mutex
	var packagePaths []string

	_ = selectedDB.ForEach(func(pkg types.Package) error {
		g.Go(func() error {
			defer func() {
				if bar != nil {
					bar.Increment()
				}
			}()

			var errs error
			for _, pkgURL := range util.Shuffle(pkg.URLs) {
				slog.Debug("Downloading package", slog.String("url", pkgURL))

				packagePath, err := downloadPackage(ctx, tempDir, pkgURL, pkg.SHA256)
				errs = errors.Join(errs, err)
				if err == nil {
					packagePathsMu.Lock()
					packagePaths = append(packagePaths, packagePath)
					packagePathsMu.Unlock()
					errs = nil
					break
				}
			}
			if errs != nil {
				return fmt.Errorf("failed to download package: %w", errs)
			}

			return nil
		})

		return nil
	})

	err := g.Wait()

	if bar != nil {
		if err != nil {
			bar.Abort(true)
		} else {
			bar.SetTotal(bar.Current(), true)
		}
		bar.Wait()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to download packages: %w", err)
	}

	// Sort the package filenames so that they are in a deterministic order.
	slices.Sort(packagePaths)

	return packagePaths, nil
}

func downloadPackage(ctx context.Context, downloadDir, pkgURL, sha256 string) (string, error) {
	url, err := url.Parse(pkgURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse package URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	// Read the package completely so the cache can be populated.
	hr := hashreader.NewReader(resp.Body)

	packageFile, err := os.Create(filepath.Join(downloadDir, filepath.Base(url.Path)))
	if err != nil {
		return "", fmt.Errorf("failed to create package file: %w", err)
	}
	defer packageFile.Close()

	if _, err := io.Copy(packageFile, hr); err != nil {
		_ = packageFile.Close()
		return "", fmt.Errorf("failed to read package: %w", err)
	}

	if err := hr.Verify(sha256); err != nil {
		_ = packageFile.Close()
		return "", fmt.Errorf("failed to verify package: %w", err)
	}

	return packageFile.Name(), nil
}

func toOCIImageConfig(recipe *latestrecipe.Recipe) ocispecs.ImageConfig {
	if recipe.Container == nil {
		return ocispecs.ImageConfig{}
	}

	return ocispecs.ImageConfig{
		User:         recipe.Container.User,
		ExposedPorts: recipe.Container.ExposedPorts,
		Env:          recipe.Container.Env,
		Entrypoint:   recipe.Container.Entrypoint,
		Cmd:          recipe.Container.Cmd,
		Volumes:      recipe.Container.Volumes,
		WorkingDir:   recipe.Container.WorkingDir,
		Labels:       recipe.Container.Labels,
		StopSignal:   recipe.Container.StopSignal,
	}
}
