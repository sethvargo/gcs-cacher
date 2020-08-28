// Package cacher defines utilities for saving and restoring caches from Google
// Cloud storage.
package cacher

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/api/option"
)

const (
	contentType  = "application/gzip"
	cacheControl = "public,max-age=3600"
)

// Cacher is responsible for saving and restoring caches.
type Cacher struct {
	client *storage.Client
}

// New creates a new cacher capable of saving and restoring the cache.
func New(ctx context.Context) (*Cacher, error) {
	client, err := storage.NewClient(ctx,
		option.WithUserAgent("gcs-cacher/1.0"))
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return &Cacher{
		client: client,
	}, nil
}

// SaveRequest is used as input to the Save operation.
type SaveRequest struct {
	// Bucket is the name of the bucket from which to cache.
	Bucket string

	// Key is the cache key.
	Key string

	// Dir is the directory on disk to cache.
	Dir string
}

// Save caches the given directory in storage.
func (c *Cacher) Save(ctx context.Context, i *SaveRequest) (retErr error) {
	if i == nil {
		retErr = fmt.Errorf("missing cache options")
		return
	}

	bucket := i.Bucket
	if bucket == "" {
		retErr = fmt.Errorf("missing bucket")
		return
	}

	dir := i.Dir
	if dir == "" {
		retErr = fmt.Errorf("missing directory")
		return
	}

	key := i.Key
	if key == "" {
		retErr = fmt.Errorf("missing key")
		return
	}

	// Create the storage writer
	gcsw := c.client.Bucket(bucket).Object(key).NewWriter(ctx)
	gcsw.ObjectAttrs.ContentType = contentType
	gcsw.ObjectAttrs.CacheControl = cacheControl
	defer func() {
		if cerr := gcsw.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gcs writer: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gcs writer: %w", cerr)
		}
	}()

	// Create the gzip writer
	gzw, err := gzip.NewWriterLevel(gcsw, gzip.BestCompression)
	if err != nil {
		retErr = fmt.Errorf("failed to create gzip writer: %w", err)
		return
	}
	defer func() {
		if cerr := gzw.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gzip writer: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gzip writer: %w", cerr)
		}
	}()

	// Create the tar writer
	tw := tar.NewWriter(gzw)
	defer func() {
		if cerr := tw.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close tar writer: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close tar writer: %w", cerr)
		}
	}()

	// Walk all files create tar
	if err := filepath.Walk(dir, func(name string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !f.Mode().IsRegular() {
			return nil
		}

		// Create the tar header
		header, err := tar.FileInfoHeader(f, f.Name())
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", f.Name(), err)
		}
		header.Name = strings.TrimPrefix(strings.Replace(name, dir, "", -1), string(filepath.Separator))

		// Write header to tar
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", f.Name(), err)
		}

		// Open and write file to tar
		file, err := os.Open(name)
		if err != nil {
			return fmt.Errorf("failed to open: %w", err)
		}

		if _, err := io.Copy(tw, file); err != nil {
			if cerr := file.Close(); cerr != nil {
				return fmt.Errorf("failed to close: %v: failed to write tar: %w", cerr, err)
			}
			return fmt.Errorf("failed to write tar: %w", err)
		}

		// Close tar
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close: %w", err)
		}

		return nil
	}); err != nil {
		retErr = fmt.Errorf("failed to walk files: %w", err)
		return
	}

	return
}

// RestoreRequest is used as input to the Restore operation.
type RestoreRequest struct {
	// Bucket is the name of the bucket from which to cache.
	Bucket string

	// Keys is the ordered list of keys to restore.
	Keys []string

	// Dir is the directory on disk to cache.
	Dir string
}

// Restore restores the key from the cache into the dir on disk.
func (c *Cacher) Restore(ctx context.Context, i *RestoreRequest) (retErr error) {
	if i == nil {
		retErr = fmt.Errorf("missing cache options")
		return
	}

	bucket := i.Bucket
	if bucket == "" {
		retErr = fmt.Errorf("missing bucket")
		return
	}

	dir := i.Dir
	if dir == "" {
		retErr = fmt.Errorf("missing directory")
		return
	}

	keys := i.Keys
	if len(keys) < 1 {
		retErr = fmt.Errorf("expected at least one cache key")
		return
	}

	// Get the bucket handle
	bucketHandle := c.client.Bucket(bucket)

	// Try to find one of the cached items
	var match *storage.ObjectAttrs
	for _, key := range keys {
		attrs, err := bucketHandle.Object(key).Attrs(ctx)
		if err != nil {
			if err == storage.ErrObjectNotExist {
				continue
			}

			retErr = fmt.Errorf("failed to list attributes for %s: %w", key, err)
			return
		}

		if match == nil || attrs.Updated.After(match.Updated) {
			match = attrs
			continue
		}
	}

	// Ensure we found one
	if match == nil {
		retErr = fmt.Errorf("failed to find cached objects among keys %q", keys)
		return
	}

	// Ensure the output directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		retErr = fmt.Errorf("failed to make target directory: %w", err)
		return
	}

	// Create the gcs reader
	gcsr, err := bucketHandle.Object(match.Name).NewReader(ctx)
	if err != nil {
		retErr = fmt.Errorf("failed to create object reader: %w", err)
		return
	}
	defer func() {
		if cerr := gcsr.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gcs reader: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gcs reader: %w", cerr)
		}
	}()

	// Create the gzip reader
	gzr, err := gzip.NewReader(gcsr)
	if err != nil {
		retErr = fmt.Errorf("failed to create gzip reader: %w", err)
		return
	}
	defer func() {
		if cerr := gzr.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gzip reader: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gzip reader: %w", cerr)
		}
	}()

	// Create the tar reader
	tr := tar.NewReader(gzr)

	// Unzip and untar each file into the target directory
	if err := func() error {
		for {
			header, err := tr.Next()
			if err != nil {
				if err == io.EOF {
					// No more files
					return nil
				}

				return fmt.Errorf("failed to read header: %w", err)
			}

			// Not entirely sure how this happens? I think it was because I uploaded a
			// bad tarball. Nonetheless, we shall check.
			if header == nil {
				continue
			}

			target := filepath.Join(dir, header.Name)

			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.MkdirAll(target, 0755); err != nil {
					return fmt.Errorf("failed to make directory: %w", err)
				}
			case tar.TypeReg:
				// Create the parent directory in case it does not exist...
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					return fmt.Errorf("failed to make parent directory: %w", err)
				}

				f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
				if err != nil {
					return fmt.Errorf("failed to open: %w", err)
				}

				if _, err := io.Copy(f, tr); err != nil {
					if cerr := f.Close(); cerr != nil {
						return fmt.Errorf("failed to close: %v: failed to untar: %w", cerr, err)
					}
					return fmt.Errorf("failed to untar: %w", err)
				}

				// Close f here instead of deferring
				if err := f.Close(); err != nil {
					return fmt.Errorf("failed to close: %w", err)
				}
			default:
				return fmt.Errorf("unknown header type %v for %s", header.Typeflag, target)
			}
		}
	}(); err != nil {
		retErr = fmt.Errorf("failed to download file: %w", err)
		return
	}

	return
}

// HashGlob hashes the files matched by the given glob.
func HashGlob(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to glob: %w", err)
	}
	return HashFiles(matches)
}

// HashFiles hashes the list of file and returns the hex-encoded SHA256.
func HashFiles(files []string) (string, error) {
	h, err := blake2b.New(16, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create hash: %w", err)
	}

	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			if cerr := f.Close(); cerr != nil {
				return "", fmt.Errorf("failed to close: %v: failed to open file: %w", cerr, err)
			}
			return "", fmt.Errorf("failed to open file: %w", err)
		}

		if _, err := io.Copy(h, f); err != nil {
			return "", fmt.Errorf("failed to hash: %w", err)
		}

		if err := f.Close(); err != nil {
			return "", fmt.Errorf("failed to close: %w", err)
		}
	}

	dig := h.Sum(nil)
	return fmt.Sprintf("%x", dig), nil
}
