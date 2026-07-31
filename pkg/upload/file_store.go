package upload

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrFileTooLarge = errors.New("file exceeds configured size limit")

type File struct {
	Key       string
	LocalPath string
	Size      int64
	ModTime   time.Time
}

type FileStore interface {
	SaveTemp(context.Context, io.Reader, int64) (File, error)
	Promote(context.Context, string, string) error
	Delete(context.Context, string) error
	List(context.Context, string) ([]File, error)
}

type LocalFileStore struct {
	root string
}

func NewLocalFileStore(root string) (*LocalFileStore, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve upload root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create upload root: %w", err)
	}
	return &LocalFileStore{root: absoluteRoot}, nil
}

func (s *LocalFileStore) SaveTemp(ctx context.Context, reader io.Reader, maxBytes int64) (File, error) {
	name, err := randomName()
	if err != nil {
		return File{}, err
	}
	key := filepath.ToSlash(filepath.Join(".tmp", name+".pdf"))
	path, err := s.resolve(key)
	if err != nil {
		return File{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return File{}, fmt.Errorf("create temporary upload directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return File{}, fmt.Errorf("create temporary upload: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()

	written, err := copyWithContext(ctx, file, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return File{}, fmt.Errorf("write temporary upload: %w", err)
	}
	if written > maxBytes {
		return File{}, ErrFileTooLarge
	}
	if err := file.Sync(); err != nil {
		return File{}, fmt.Errorf("sync temporary upload: %w", err)
	}
	if err := file.Close(); err != nil {
		return File{}, fmt.Errorf("close temporary upload: %w", err)
	}
	remove = false
	return File{Key: key, LocalPath: path, Size: written, ModTime: time.Now()}, nil
}

func (s *LocalFileStore) Promote(_ context.Context, temporaryKey, finalKey string) error {
	temporaryPath, err := s.resolve(temporaryKey)
	if err != nil {
		return err
	}
	finalPath, err := s.resolve(finalKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
		return fmt.Errorf("create final upload directory: %w", err)
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("promote uploaded file: %w", err)
	}
	return nil
}

func (s *LocalFileStore) Delete(_ context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete uploaded file: %w", err)
	}
	return nil
}

func (s *LocalFileStore) List(ctx context.Context, prefix string) ([]File, error) {
	start, err := s.resolve(prefix)
	if err != nil {
		return nil, err
	}
	files := make([]File, 0)
	err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) && path == start {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		files = append(files, File{Key: filepath.ToSlash(relative), LocalPath: path, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list uploaded files: %w", err)
	}
	return files, nil
}

func DocumentKey(userID int64, now time.Time) (string, error) {
	name, err := randomName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("documents/%d/%04d/%02d/%s.pdf", userID, now.Year(), int(now.Month()), name), nil
}

func (s *LocalFileStore) resolve(key string) (string, error) {
	if key == "" {
		return "", errors.New("empty storage key")
	}
	cleanKey := filepath.Clean(filepath.FromSlash(key))
	if filepath.IsAbs(cleanKey) || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid storage key")
	}
	path := filepath.Join(s.root, cleanKey)
	relative, err := filepath.Rel(s.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("storage key escapes upload root")
	}
	return path, nil
}

func randomName() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate upload key: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			count, writeErr := destination.Write(buffer[:read])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
