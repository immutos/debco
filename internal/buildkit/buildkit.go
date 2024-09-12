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

package buildkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd/platforms"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"

	"github.com/immutos/debco/internal/buildkit/exptypes"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/progress/progresswriter"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

// BuildKit is a wrapper around BuildKit that provides a simplified interface
// for building OCI images using BuildKit running in a Docker container.
type BuildKit struct {
	certsDir      string
	containerName string
	address       string
}

// New creates a new BuildKit instance.
func New(name, certsDir string) *BuildKit {
	return &BuildKit{
		containerName: fmt.Sprintf("%s-buildkitd", name),
		certsDir:      certsDir,
	}
}

type BuildOptions struct {
	// OCIArchivePath is the path to the output OCI image tarball.
	OCIArchivePath string
	// RecipePath is the path to the debco recipe file.
	RecipePath string
	// SourceDateEpoch is the source date epoch for the image.
	SourceDateEpoch time.Time
	// SecondStageBinaryPath optionally overrides the path to the second-stage binary.
	SecondStageBinaryPath string
	// ImageConf is the optional OCI image configuration.
	ImageConf ocispecs.ImageConfig
	// Tags is a list of tags to apply to the image.
	Tags []string
	// PlatformOpts is a list of platform build options.
	PlatformOpts []PlatformBuildOptions
}

type PlatformBuildOptions struct {
	// Platform is the platform to build the image for.
	Platform ocispecs.Platform
	// BuildContextDir is the path to the build context directory.
	BuildContextDir string
	// DpkgDatabaseArchivePath is the path to the dpkg configuration archive.
	// The path must be relative to the build context directory.
	DpkgDatabaseArchivePath string
	// DataArchivePaths is a list of paths to package data archives.
	// The paths must be relative to the build context directory.
	DataArchivePaths []string
}

// Build builds an OCI image tarball using BuildKit.
func (b *BuildKit) Build(ctx context.Context, opts BuildOptions) error {
	isMultiPlatform := len(opts.PlatformOpts) > 1

	buildFunc := func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
		res := gateway.NewResult()

		for _, platformOpt := range opts.PlatformOpts {
			platformStr := platforms.Format(platforms.Normalize(platformOpt.Platform))

			buildContextKey := fmt.Sprintf("build-context-%s", strings.ReplaceAll(platformStr, "/", "-"))

			dpkgDatabaseArchiveRelPath, err := filepath.Rel(platformOpt.BuildContextDir, platformOpt.DpkgDatabaseArchivePath)
			if err != nil {
				return nil, fmt.Errorf("failed to get relative path to dpkg configuration archive: %w", err)
			}

			// Create an LLB definition for the build.
			state := llb.Scratch().
				Platform(platforms.Normalize(platformOpt.Platform)).
				AddEnv("DEBIAN_FRONTEND", "noninteractive").
				AddEnv("DEBCONF_NONINTERACTIVE_SEEN", "true").
				AddEnv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
				File(llb.Copy(llb.Local(buildContextKey), dpkgDatabaseArchiveRelPath, "/", &llb.CopyInfo{AttemptUnpack: true}))

			for _, dataArchivePath := range platformOpt.DataArchivePaths {
				dataArchiveRelPath, err := filepath.Rel(platformOpt.BuildContextDir, dataArchivePath)
				if err != nil {
					return nil, fmt.Errorf("failed to get relative path to data archive: %w", err)
				}

				state = state.File(llb.Copy(llb.Local(buildContextKey), dataArchiveRelPath, "/", &llb.CopyInfo{AttemptUnpack: true}))
			}

			if opts.SecondStageBinaryPath != "" {
				// Copy the debco binary into the root filesystem.
				state = state.File(llb.Copy(llb.Local("second-stage-bin"), filepath.Base(opts.SecondStageBinaryPath), "/usr/bin/debco", &llb.CopyInfo{}))
			}

			state = state.
				Run(llb.Shlex("debco second-stage merge-usr")).                   // Merge the /usr directory into the root filesystem.
				Run(llb.Shlex("/var/lib/dpkg/info/base-passwd.preinst install")). // Create the /etc/group and /etc/passwd files (needed by dpkg).
				Run(llb.Shlex("dpkg --configure -a")).                            // Configure the packages.
				// Remove the dpkg log file, alternatives log file, and ldconfig cache file.
				// These files are no longer needed and will lead to irreproducible builds.
				File(llb.Rm("/var/log/dpkg.log")).
				File(llb.Rm("/var/log/alternatives.log")).
				File(llb.Rm("/var/cache/ldconfig/aux-cache"))

			// Provision image (eg. create users/groups etc).
			state = state.
				File(llb.Copy(llb.Local("conf"), filepath.Base(opts.RecipePath), "/etc/debco/config.yaml", &llb.CopyInfo{CreateDestPath: true})).
				Run(llb.Shlex("debco second-stage provision -f /etc/debco/config.yaml")).
				Root().
				File(llb.Rm("/etc/debco"))

			// Remove the no longer needed debco binary.
			if opts.SecondStageBinaryPath != "" {
				state = state.File(llb.Rm("/usr/bin/debco"))
			} else {
				state = state.Run(llb.Shlex("dpkg -r debco")).
					Root()
			}

			// Squash everything into a single final layer.
			state = llb.Scratch().
				File(llb.Copy(state, "/", "/", &llb.CopyInfo{}))

			// Marshal the LLB definition.
			def, err := state.Marshal(ctx, llb.Platform(platformOpt.Platform))
			if err != nil {
				return nil, err
			}

			r, err := c.Solve(ctx, gateway.SolveRequest{
				Definition: def.ToPB(),
			})
			if err != nil {
				return nil, err
			}

			ref, err := r.SingleRef()
			if err != nil {
				return nil, err
			}

			_, err = ref.ToState()
			if err != nil {
				return nil, err
			}

			imageConfBytes, err := exporterImageConfig(opts.ImageConf, platformOpt)
			if err != nil {
				return nil, err
			}

			if isMultiPlatform {
				res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageConfigKey, platformStr), imageConfBytes)
				res.AddRef(platformStr, ref)
			} else {
				res.AddMeta(exptypes.ExporterImageConfigKey, imageConfBytes)
				res.SetRef(ref)
			}
		}

		res.AddMeta(exptypes.ExporterPlatformsKey, exporterPlatforms(opts.PlatformOpts...))

		return res, nil
	}

	buildkitURL, err := url.Parse(b.address)
	if err != nil {
		return fmt.Errorf("failed to parse buildkit address: %w", err)
	}

	c, err := client.New(ctx, "buildkitd", client.WithCredentials("buildkitd",
		filepath.Join(b.certsDir, "ca.pem"), filepath.Join(b.certsDir, "debco.pem"), filepath.Join(b.certsDir, "debco-key.pem")),
		client.WithContextDialer(func(_ context.Context, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", buildkitURL.Host)
		}))
	if err != nil {
		return fmt.Errorf("failed to create buildkit client: %w", err)
	}
	defer c.Close()

	// Create a new session.
	sess, err := session.NewSession(ctx, "debco", identity.NewID())
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()

	mode := "auto"
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		mode = "plain"
	}

	printerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	pw, err := progresswriter.NewPrinter(printerCtx, os.Stdout, mode)
	if err != nil {
		return fmt.Errorf("failed to create progress writer: %w", err)
	}
	defer func() {
		<-pw.Done()

		if err := pw.Err(); err != nil {
			slog.Warn("Failed to write progress", slog.Any("error", err))
		}
	}()

	localDirs := map[string]string{
		"conf": filepath.Dir(opts.RecipePath),
	}

	if opts.SecondStageBinaryPath != "" {
		localDirs["second-stage-bin"] = filepath.Dir(opts.SecondStageBinaryPath)
	}

	for _, platformOpt := range opts.PlatformOpts {
		platformStr := platforms.Format(platforms.Normalize(platformOpt.Platform))

		buildContextKey := fmt.Sprintf("build-context-%s", strings.ReplaceAll(platformStr, "/", "-"))
		localDirs[buildContextKey] = platformOpt.BuildContextDir
	}

	_, err = c.Build(ctx, client.SolveOpt{
		LocalDirs: localDirs,
		Exports: []client.ExportEntry{
			{
				Type: client.ExporterOCI,
				Output: func(_ map[string]string) (io.WriteCloser, error) {
					ociArchiveFile, err := os.Create(opts.OCIArchivePath)
					if err != nil {
						return nil, fmt.Errorf("failed to create output oci tarball: %w", err)
					}

					return ociArchiveFile, nil
				},
				Attrs: map[string]string{
					"name":                          strings.Join(opts.Tags, ","),
					exptypes.OptKeySourceDateEpoch:  strconv.Itoa(int(opts.SourceDateEpoch.UTC().Unix())),
					exptypes.OptKeyRewriteTimestamp: "true",
				},
			},
		},
	}, "", buildFunc, pw.Status())
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	return nil
}

func exporterPlatforms(platformOpts ...PlatformBuildOptions) []byte {
	exporterPlatforms := exptypes.Platforms{
		Platforms: make([]exptypes.Platform, len(platformOpts)),
	}

	for i, p := range platformOpts {
		exporterPlatforms.Platforms[i] = exptypes.Platform{
			ID:       platforms.Format(platforms.Normalize(p.Platform)),
			Platform: platforms.Normalize(p.Platform),
		}
	}

	exporterPlatformsBytes, err := json.Marshal(&exporterPlatforms)
	if err != nil {
		panic(err)
	}

	return exporterPlatformsBytes
}

func exporterImageConfig(imageConf ocispecs.ImageConfig, platformOpt PlatformBuildOptions) ([]byte, error) {
	defaultEnv := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=xterm",
	}

	imageConf.Env = append(defaultEnv, imageConf.Env...)

	img := ocispecs.Image{
		Platform: platformOpt.Platform,
		Config:   imageConf,
		RootFS: ocispecs.RootFS{
			Type: "layers",
		},
	}

	data, err := json.Marshal(img)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal image config: %w", err)
	}

	return data, nil
}
