// +build windows

package fs

import (
	"context"
	"os"
	"path/filepath"
)

func diskUsage(roots ...string) (Usage, error) {
	var (
		size int64
	)

	// TODO (stevvooe): Support inodes (or equivalent) for windows. id:377 gh:378

	for _, root := range roots {
		if err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			size += fi.Size()
			return nil
		}); err != nil {
			return Usage{}, err
		}
	}

	return Usage{
		Size: size,
	}, nil
}

func diffUsage(ctx context.Context, a, b string) (Usage, error) {
	var (
		size int64
	)

	if err := Changes(ctx, a, b, func(kind ChangeKind, _ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if kind == ChangeKindAdd || kind == ChangeKindModify {
			size += fi.Size()

			return nil

		}
		return nil
	}); err != nil {
		return Usage{}, err
	}

	return Usage{
		Size: size,
	}, nil
}
