package pkg

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCopyChown parses a COPY/ADD --chown value into tar options.
//
// Supported forms (docker-compatible subset):
//
//	"uid:gid"   numeric ids, e.g. 1000:1000
//	"uid"       numeric uid, gid unset
//	":gid"      gid only
//	"root[:x]"  the root user/group resolves to 0
//
// Named users other than root cannot be resolved without running inside the
// target rootfs, so they are rejected with a hint instead of silently
// producing wrong ownership.
func parseCopyChown(s string) ([]TarOption, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	user, group, hasGroup := strings.Cut(s, ":")
	resolve := func(part, what string) (*int, error) {
		if part == "" {
			return nil, nil
		}
		if id, err := strconv.Atoi(part); err == nil {
			return &id, nil
		}
		if part == "root" {
			zero := 0
			return &zero, nil
		}
		return nil, fmt.Errorf("cannot resolve %s %q without the target rootfs; use a numeric id, e.g. --chown=1000:1000", what, part)
	}

	uid, err := resolve(user, "user")
	if err != nil {
		return nil, err
	}
	var gid *int
	if hasGroup {
		if gid, err = resolve(group, "group"); err != nil {
			return nil, err
		}
	}

	var options []TarOption
	switch {
	case uid != nil && gid != nil:
		options = append(options, ChownOption(*uid, *gid))
	case uid != nil:
		options = append(options, ChownUIDOption(*uid))
	case gid != nil:
		options = append(options, ChownGIDOption(*gid))
	}
	return options, nil
}

// parseCopyChmod parses a COPY/ADD --chmod octal value (e.g. "755", "0755").
// Like docker, the mode is applied verbatim to files and directories alike.
func parseCopyChmod(s string) (TarOption, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	mod, err := strconv.ParseInt(s, 8, 64)
	if err != nil || mod < 0 || mod > 0o777 {
		return nil, fmt.Errorf("invalid --chmod %q: expected octal like 755 or 0755", s)
	}
	return ChmodOption(mod), nil
}
