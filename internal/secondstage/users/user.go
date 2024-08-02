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

package users

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	systemUIDMin uint = 100
	systemUIDMax uint = 999
	userUIDMin   uint = 1000
	userUIDMax   uint = 60000
)

var (
	// For testing.
	passwdFilePath = "/etc/passwd"
	shadowFilePath = "/etc/shadow"
)

type User struct {
	Name     string
	UID      uint
	Groups   []string
	HomeDir  string
	Shell    string
	Password string
	System   bool
}

func CreateOrUpdateUser(user User) error {
	if !validNameRegexp.MatchString(user.Name) {
		return fmt.Errorf("invalid user name %q", user.Name)
	}

	if len(user.Groups) == 0 {
		return fmt.Errorf("user %q must belong to at least one group", user.Name)
	}

	groups, err := loadGroups()
	if err != nil {
		return fmt.Errorf("failed to load groups: %w", err)
	}

	lookupGroup := func(groupName string) (Group, error) {
		var gid uint
		if id, err := strconv.Atoi(groupName); err == nil {
			gid = uint(id)
		} else {
			var found bool
			for _, grp := range groups {
				if grp.Name == groupName {
					gid = grp.GID
					found = true
					break
				}
			}
			if !found {
				return Group{}, fmt.Errorf("group %q not found", groupName)
			}
		}

		return groups[gid], nil
	}

	primaryGroup, err := lookupGroup(user.Groups[0])
	if err != nil {
		return fmt.Errorf("failed to lookup primary group: %w", err)
	}

	if user.UID == 0 {
		var err error
		user.UID, err = getNextFreeUID(user.System)
		if err != nil {
			return err
		}
	}

	if err := updatePasswdFile(user, primaryGroup.GID); err != nil {
		return fmt.Errorf("failed to update passwd: %w", err)
	}

	if err := updateShadowFile(user); err != nil {
		return fmt.Errorf("failed to update shadow: %w", err)
	}

	for _, groupName := range user.Groups {
		group, err := lookupGroup(groupName)
		if err != nil {
			return fmt.Errorf("failed to lookup group %q: %w", groupName, err)
		}

		group.Members = append(group.Members, user.Name)

		if err := CreateOrUpdateGroup(group); err != nil {
			return fmt.Errorf("failed to update group: %w", err)
		}
	}

	return nil
}

func getNextFreeUID(system bool) (uint, error) {
	users, err := loadUsers()
	if err != nil {
		return 0, fmt.Errorf("failed to parse passwd file: %w", err)
	}

	minUID := userUIDMin
	if system {
		minUID = systemUIDMin
	}

	for uid := minUID; uid <= userUIDMax; uid++ {
		if _, exists := users[uid]; !exists {
			return uid, nil
		}
	}

	return 0, errors.New("no available UID")

}

func loadUsers() (map[uint]User, error) {
	f, err := os.Open(passwdFilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	users := make(map[uint]User)

	lr := &lineReader{bufio.NewReader(f)}
	for {
		line, err := lr.nextLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		// Skip comments.
		if line[0] == '#' {
			continue
		}

		fields := strings.Split(line, ":")
		if len(fields) < 6 {
			return nil, fmt.Errorf("invalid user entry: %q", line)
		}

		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("invalid UID: %w", err)
		}

		users[uint(uid)] = User{
			Name:    fields[0],
			UID:     uint(uid),
			HomeDir: fields[5],
			Shell:   fields[6],
		}
	}

	return users, nil
}

func updatePasswdFile(user User, primaryGroupID uint) error {
	updateFunc := func(lr *lineReader) (string, error) {
		updatedEntry := fmt.Sprintf("%s:x:%d:%d::%s:%s", user.Name, user.UID, primaryGroupID, user.HomeDir, user.Shell)
		found := false

		var sb strings.Builder
		for {
			line, err := lr.nextLine()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				return "", err
			}

			if strings.HasPrefix(line, user.Name+":") {
				sb.WriteString(updatedEntry)
				found = true
			} else {
				sb.WriteString(line)
			}
			sb.WriteRune('\n')
		}
		if !found {
			sb.WriteString(updatedEntry)
			sb.WriteRune('\n')
		}

		return sb.String(), nil
	}

	return updateFile(passwdFilePath, 0o644, updateFunc)
}

func updateShadowFile(user User) error {
	updateFunc := func(lr *lineReader) (string, error) {
		passwordHash := "!"
		if user.Password != "" {
			// Ideally we would use yescrypt but there is no good Go implementations.
			hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
			if err != nil {
				return "", fmt.Errorf("failed to hash password: %w", err)
			}

			passwordHash = string(hash)
		}

		// Just a random fixed epoch.
		updatedEntry := fmt.Sprintf("%s:%s:19928:0:99999:7:::", user.Name, passwordHash)
		found := false

		var sb strings.Builder
		for {
			line, err := lr.nextLine()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				return "", err
			}

			if strings.HasPrefix(string(line), user.Name+":") {
				sb.WriteString(updatedEntry)
				found = true
			} else {
				sb.WriteString(line)
			}
			sb.WriteRune('\n')

		}
		if !found {
			sb.WriteString(updatedEntry)
			sb.WriteRune('\n')
		}

		return sb.String(), nil
	}

	// Do we have a shadow file?
	if _, err := os.Stat(shadowFilePath); os.IsNotExist(err) {
		if user.Password != "" {
			return fmt.Errorf("shadow files are required for password hashes: %w", err)
		}

		slog.Warn("No shadow file found, skipping")
		return nil
	}

	return updateFile(shadowFilePath, 0o400, updateFunc)
}
