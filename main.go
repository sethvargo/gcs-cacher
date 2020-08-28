package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sethvargo/gcs-cacher/cacher"
)

var (
	stdout = os.Stdout
	stderr = os.Stderr

	// bucket is the Cloud Storage bucket.
	bucket string

	// cache is the key to use to cache.
	cache string

	// restore is the list of restore keys to use to restore.
	restore stringSliceFlag

	// dir is the directory on disk to cache or the destination in which to
	// restore.
	dir string

	// hash is the glob pattern to hash.
	hash string
)

func init() {
	flag.StringVar(&bucket, "bucket", "", "Bucket name without gs:// prefix.")
	flag.StringVar(&dir, "dir", "", "Directory to cache or restore.")

	flag.StringVar(&cache, "cache", "", "Key with which to cache.")
	flag.Var(&restore, "restore", "Keys to search to restore (can use multiple times).")
	flag.StringVar(&hash, "hash", "", "Glob pattern to hash.")
}

func main() {
	args := os.Args
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			flag.PrintDefaults()
			os.Exit(0)
		}
	}

	flag.Parse()
	if len(flag.Args()) > 0 {
		fmt.Fprintf(stderr, "no arguments expected\n")
		os.Exit(1)
	}

	switch {
	case cache != "":
		if err := saveCache(bucket, dir, cache); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			os.Exit(1)
		}
	case restore != nil:
		if err := restoreCache(bucket, dir, restore); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			os.Exit(1)
		}
	case hash != "":
		dig, err := cacher.HashGlob(hash)
		if err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
		}
		fmt.Fprintf(stdout, "%s", dig)
	default:
		fmt.Fprintf(stderr, "missing command operation!\n")
		os.Exit(1)
	}
}

func saveCache(bucket, dir, key string) error {
	ctx := context.Background()
	c, err := cacher.New(ctx)
	if err != nil {
		return err
	}

	return c.Save(ctx, &cacher.SaveRequest{
		Bucket: bucket,
		Dir:    dir,
		Key:    key,
	})
}

func restoreCache(bucket, dir string, keys []string) error {
	ctx := context.Background()
	c, err := cacher.New(ctx)
	if err != nil {
		return err
	}

	return c.Restore(ctx, &cacher.RestoreRequest{
		Bucket: bucket,
		Dir:    dir,
		Keys:   keys,
	})
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}
func (s *stringSliceFlag) Set(value string) error {
	var vals []string
	for _, val := range strings.Split(value, ",") {
		if k := strings.TrimSpace(val); k != "" {
			vals = append(vals, k)
		}
	}
	*s = append(*s, vals...)
	return nil
}
