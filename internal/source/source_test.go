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

package source_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpeckett/deb822/types/arch"
	latestrecipe "github.com/dpeckett/debco/internal/recipe/v1alpha1"
	"github.com/dpeckett/debco/internal/source"
	"github.com/dpeckett/debco/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSource(t *testing.T) {
	testutil.SetupGlobals(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	resultCh := make(chan runMirrorResult, 1)
	t.Cleanup(func() {
		close(resultCh)
	})

	go runDebianMirror(ctx, resultCh)

	mirrorResult := <-resultCh
	require.NoError(t, mirrorResult.err)

	s, err := source.NewSource(ctx, latestrecipe.SourceConfig{
		URL:      fmt.Sprintf("http://%s/debian", mirrorResult.addr.String()),
		SignedBy: filepath.Join(testutil.Root(), "testdata/archive-key-12.asc"),
	})
	require.NoError(t, err)

	components, err := s.Components(ctx, arch.MustParse("amd64"))
	require.NoError(t, err)

	require.Len(t, components, 2)
	require.Equal(t, "main", components[0].Name)
	require.Equal(t, "all", components[0].Arch.String())
	require.Equal(t, "main", components[1].Name)
	require.Equal(t, "amd64", components[1].Arch.String())

	componentPackages, lastUpdated, err := components[1].Packages(ctx)
	require.NoError(t, err)

	require.Len(t, componentPackages, 63408)

	require.NotEqual(t, time.Time{}, lastUpdated)
}

type runMirrorResult struct {
	err  error
	addr net.Addr
}

func runDebianMirror(ctx context.Context, result chan runMirrorResult) {
	mux := http.NewServeMux()

	rootDir := filepath.Join(testutil.Root(), "testdata")

	mux.HandleFunc("/debian/dists/stable/InRelease", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(rootDir, "InRelease"))
	})

	mux.HandleFunc("/debian/dists/stable/main/binary-amd64/Packages.gz", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(rootDir, "Packages.gz"))
	})

	srv := &http.Server{
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		result <- runMirrorResult{err: err}
		return
	}

	result <- runMirrorResult{addr: lis.Addr()}

	go func() {
		<-ctx.Done()

		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(lis); err != nil {
		return
	}
}
